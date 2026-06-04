package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	eventclientv1 "k8s.io/client-go/kubernetes/typed/events/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sundi0331/logbook/config"
)

const retryDelay = 5 * time.Second

type eventWatcher interface {
	List(ctx context.Context, opts metav1.ListOptions) (*eventsv1.EventList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

func Run(parent context.Context, cfg *config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	client, err := createEventsClient(&cfg.Auth, logger)
	if err != nil {
		return fmt.Errorf("create Kubernetes events client: %w", err)
	}

	return watchEvents(ctx, cfg, client.Events(cfg.Target.Namespace), logger)
}

func watchEvents(ctx context.Context, cfg *config.Config, events eventWatcher, logger *slog.Logger) error {
	logger.Info("starting Kubernetes event watcher", "api_group", "events.k8s.io", "api_version", "v1", "namespace", namespaceLabel(cfg.Target.Namespace))

	options := cfg.Target.ListOptions
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
		}

		watcher, err := events.Watch(ctx, options)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logger.Error("failed to start event watch", "error", err, "retry_after", retryDelay.String())
			if err := waitForRetry(ctx); err != nil {
				return nil
			}
			continue
		}

		resourceVersion, err := consumeEvents(ctx, watcher, logger)
		watcher.Stop()
		if err != nil {
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

func consumeEvents(ctx context.Context, watcher watch.Interface, logger *slog.Logger) (string, error) {
	resourceVersion := ""
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received; stopping event watcher")
			return resourceVersion, nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return resourceVersion, nil
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
		}
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

func createEventsClient(authCfg *config.AuthConfig, logger *slog.Logger) (*eventclientv1.EventsV1Client, error) {
	restConfig, err := createRESTConfig(authCfg, logger)
	if err != nil {
		return nil, err
	}
	return eventclientv1.NewForConfig(restConfig)
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
