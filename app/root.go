package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	coordinationclientv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	eventclientv1 "k8s.io/client-go/kubernetes/typed/events/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sundi0331/logbook/config"
)

const (
	retryDelay             = 5 * time.Second
	checkpointFlushTimeout = 10 * time.Second
)

var errResourceVersionExpired = errors.New("Kubernetes event resource version expired")

type eventWatcher interface {
	List(ctx context.Context, opts metav1.ListOptions) (*eventsv1.EventList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

func Run(parent context.Context, cfg *config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	restConfig, err := createRESTConfig(&cfg.Auth, logger)
	if err != nil {
		return err
	}

	eventsClient, err := eventclientv1.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes events client: %w", err)
	}

	var coreClient *coreclientv1.CoreV1Client
	if needsCoreClient(cfg) {
		coreClient, err = coreclientv1.NewForConfig(restConfig)
		if err != nil {
			return fmt.Errorf("create Kubernetes core client: %w", err)
		}
	}
	checkpoint, err := newCheckpointStore(cfg.Checkpoint, coreClient)
	if err != nil {
		return fmt.Errorf("create checkpoint store: %w", err)
	}

	flushInterval := time.Duration(0)
	if checkpoint != nil {
		flushInterval, err = checkpointFlushInterval(cfg.Checkpoint)
		if err != nil {
			return err
		}
	}

	leaderClients := leaderElectionClients{}
	if cfg.LeaderElection.Enabled {
		coordinationClient, err := coordinationclientv1.NewForConfig(restConfig)
		if err != nil {
			return fmt.Errorf("create Kubernetes coordination client: %w", err)
		}
		leaderClients = leaderElectionClients{core: coreClient, coordination: coordinationClient}
	}

	return runWithLeaderElection(
		ctx,
		cfg.LeaderElection,
		leaderClients,
		logger,
		func(leaderCtx context.Context) error {
			return watchEvents(leaderCtx, cfg, eventsClient.Events(cfg.Target.Namespace), checkpoint, flushInterval, logger)
		},
	)
}

func needsCoreClient(cfg *config.Config) bool {
	return cfg.LeaderElection.Enabled || (cfg.Checkpoint.Enabled && cfg.Checkpoint.Backend == "configmap")
}

func watchEvents(ctx context.Context, cfg *config.Config, events eventWatcher, checkpoint checkpointStore, checkpointFlushInterval time.Duration, logger *slog.Logger) error {
	logger.Info("starting Kubernetes event watcher", "api_group", "events.k8s.io", "api_version", "v1", "namespace", namespaceLabel(cfg.Target.Namespace), "checkpoint_enabled", cfg.Checkpoint.Enabled, "checkpoint_backend", cfg.Checkpoint.Backend, "checkpoint_flush_interval", checkpointFlushInterval.String())

	options := cfg.Target.ListOptions
	if checkpoint != nil {
		resourceVersion, err := checkpoint.Load(ctx)
		if err != nil {
			return fmt.Errorf("load checkpoint: %w", err)
		}
		if resourceVersion != "" {
			options.ResourceVersion = resourceVersion
			logger.Info("loaded Kubernetes event checkpoint", "resource_version", resourceVersion)
		}
	}

	for {
		if options.ResourceVersion == "" {
			resourceVersion, err := currentResourceVersion(ctx, events, options, logger)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				logger.Error("failed to list events before watch", "error", err, "retry_after", retryDelay.String())
				if err := waitForRetry(ctx); err != nil {
					return nil
				}
				continue
			}
			options.ResourceVersion = resourceVersion
			if checkpoint != nil {
				if err := checkpoint.Save(ctx, resourceVersion); err != nil {
					return fmt.Errorf("save initialized checkpoint: %w", err)
				}
				if err := checkpoint.Flush(ctx); err != nil {
					return fmt.Errorf("flush initialized checkpoint: %w", err)
				}
			}
		}

		watcher, err := events.Watch(ctx, options)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if apierrors.IsResourceExpired(err) {
				resourceVersion, err := recoverExpiredResourceVersion(ctx, cfg, events, checkpoint, logger)
				if err != nil {
					return err
				}
				options.ResourceVersion = resourceVersion
				continue
			}
			logger.Error("failed to start event watch", "error", err, "retry_after", retryDelay.String())
			if err := waitForRetry(ctx); err != nil {
				return nil
			}
			continue
		}

		resourceVersion, err := consumeEvents(ctx, watcher, checkpoint, checkpointFlushInterval, logger)
		watcher.Stop()
		if err != nil {
			if errors.Is(err, errResourceVersionExpired) {
				resourceVersion, err := recoverExpiredResourceVersion(ctx, cfg, events, checkpoint, logger)
				if err != nil {
					return err
				}
				options.ResourceVersion = resourceVersion
				continue
			}
			return err
		}
		if resourceVersion != "" {
			options.ResourceVersion = resourceVersion
		}

		logger.Warn("event watch channel closed", "retry_after", retryDelay.String(), "resource_version", options.ResourceVersion)
		if err := waitForRetry(ctx); err != nil {
			return nil
		}
	}
}

func consumeEvents(ctx context.Context, watcher watch.Interface, checkpoint checkpointStore, checkpointFlushInterval time.Duration, logger *slog.Logger) (string, error) {
	resourceVersion := ""
	var checkpointTicker *time.Ticker
	var checkpointTicks <-chan time.Time
	if checkpoint != nil && checkpointFlushInterval > 0 {
		checkpointTicker = time.NewTicker(checkpointFlushInterval)
		defer checkpointTicker.Stop()
		checkpointTicks = checkpointTicker.C
	}

	flushCheckpoint := func() error {
		if checkpoint == nil {
			return nil
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), checkpointFlushTimeout)
		defer cancel()
		if err := checkpoint.Flush(flushCtx); err != nil {
			return fmt.Errorf("flush checkpoint: %w", err)
		}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received; stopping event watcher")
			return resourceVersion, flushCheckpoint()
		case <-checkpointTicks:
			if err := flushCheckpoint(); err != nil {
				return resourceVersion, err
			}
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return resourceVersion, flushCheckpoint()
			}
			if event.Type == watch.Error {
				if apierrors.IsResourceExpired(apierrors.FromObject(event.Object)) {
					return resourceVersion, errResourceVersionExpired
				}
				marshalledEvent, err := json.Marshal(event)
				if err != nil {
					return resourceVersion, fmt.Errorf("marshal Kubernetes watch error event: %w", err)
				}
				return resourceVersion, fmt.Errorf("Kubernetes watch error event: %s", string(marshalledEvent))
			}
			if event.Object != nil {
				accessor, err := meta.Accessor(event.Object)
				if err == nil {
					resourceVersion = accessor.GetResourceVersion()
				}
			}

			marshalledEvent, err := json.Marshal(event)
			if err != nil {
				return resourceVersion, fmt.Errorf("marshal Kubernetes watch event: %w", err)
			}
			logger.Info("kubernetes event observed", "watch_type", string(event.Type), "event", json.RawMessage(marshalledEvent), "resource_version", resourceVersion)
			if checkpoint != nil && resourceVersion != "" {
				if err := checkpoint.Save(ctx, resourceVersion); err != nil {
					return resourceVersion, fmt.Errorf("save checkpoint: %w", err)
				}
			}
		}
	}
}

func recoverExpiredResourceVersion(ctx context.Context, cfg *config.Config, events eventWatcher, checkpoint checkpointStore, logger *slog.Logger) (string, error) {
	switch cfg.Checkpoint.OnExpiredResourceVersion {
	case "skip-existing":
		logger.Warn("Kubernetes event resource version expired; advancing checkpoint to current resource version")
		resourceVersion, err := currentResourceVersion(ctx, events, cfg.Target.ListOptions, logger)
		if err != nil {
			if ctx.Err() != nil {
				return "", nil
			}
			return "", fmt.Errorf("recover expired resource version: %w", err)
		}
		if checkpoint != nil {
			if err := checkpoint.Save(ctx, resourceVersion); err != nil {
				return "", fmt.Errorf("save recovered checkpoint: %w", err)
			}
			if err := checkpoint.Flush(ctx); err != nil {
				return "", fmt.Errorf("flush recovered checkpoint: %w", err)
			}
		}
		return resourceVersion, nil
	case "fail":
		return "", errResourceVersionExpired
	default:
		return "", fmt.Errorf("unsupported checkpoint expiration policy %q: expected skip-existing or fail", cfg.Checkpoint.OnExpiredResourceVersion)
	}
}

func currentResourceVersion(ctx context.Context, events eventWatcher, options metav1.ListOptions, logger *slog.Logger) (string, error) {
	list, err := events.List(ctx, options)
	if err != nil {
		return "", err
	}
	logger.Info("initialized Kubernetes event resource version", "resource_version", list.ResourceVersion, "events_count", len(list.Items))
	return list.ResourceVersion, nil
}

func waitForRetry(ctx context.Context) error {
	timer := time.NewTimer(retryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func createRESTConfig(authCfg *config.AuthConfig, logger *slog.Logger) (*rest.Config, error) {
	switch authCfg.Mode {
	case "in-cluster":
		logger.Info("starting in in-cluster mode")
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("load in-cluster config: %w", err)
		}
		return config, nil
	case "out-of-cluster":
		logger.Info("starting in out-of-cluster mode")
		kubeConfig := authCfg.KubeConfig
		if kubeConfig == "" {
			logger.Info("kubeconfig not provided; using default path")
			if home := homeDir(); home != "" {
				kubeConfig = filepath.Join(home, ".kube", "config")
			}
		}
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig %q: %w", kubeConfig, err)
		}
		return config, nil
	default:
		return nil, fmt.Errorf("unsupported auth mode %q: expected in-cluster or out-of-cluster", authCfg.Mode)
	}
}

func namespaceLabel(namespace string) string {
	if namespace == "" {
		return "all"
	}
	return namespace
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}
