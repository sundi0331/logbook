package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"k8s.io/client-go/kubernetes/typed/coordination/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/sundi0331/logbook/config"
)

type leaderElectionClients struct {
	core         coreclientv1.CoreV1Interface
	coordination v1.CoordinationV1Interface
}

func runWithLeaderElection(ctx context.Context, cfg config.LeaderElectionConfig, clients leaderElectionClients, logger *slog.Logger, run func(context.Context) error) error {
	if !cfg.Enabled {
		return run(ctx)
	}

	identity, err := leaderElectionIdentity(cfg)
	if err != nil {
		return err
	}
	namespace := leaderElectionNamespace(cfg)
	name := cfg.Name

	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace,
		name,
		clients.core,
		clients.coordination,
		resourcelock.ResourceLockConfig{Identity: identity},
	)
	if err != nil {
		return fmt.Errorf("create leader election lock: %w", err)
	}

	timing, err := leaderElectionTiming(cfg)
	if err != nil {
		return err
	}

	electionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	runStarted := make(chan struct{}, 1)
	runDone := make(chan error, 1)
	leaderConfig := leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: timing.leaseDuration,
		RenewDeadline: timing.renewDeadline,
		RetryPeriod:   timing.retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				logger.Info("acquired leader election lease", "leader_identity", identity, "lease_namespace", namespace, "lease_name", name)
				select {
				case runStarted <- struct{}{}:
				default:
				}
				err := run(leaderCtx)
				runDone <- err
				cancel()
			},
			OnStoppedLeading: func() {
				logger.Info("stopped leading", "leader_identity", identity, "lease_namespace", namespace, "lease_name", name)
			},
			OnNewLeader: func(current string) {
				if current == identity {
					return
				}
				logger.Info("observed leader election holder", "leader_identity", current, "lease_namespace", namespace, "lease_name", name)
			},
		},
		Name: name,
	}

	leader, err := leaderelection.NewLeaderElector(leaderConfig)
	if err != nil {
		return fmt.Errorf("configure leader election: %w", err)
	}
	leader.Run(electionCtx)

	select {
	case <-runStarted:
		return <-runDone
	default:
		return nil
	}
}

type leaderElectionDurations struct {
	leaseDuration time.Duration
	renewDeadline time.Duration
	retryPeriod   time.Duration
}

func leaderElectionTiming(cfg config.LeaderElectionConfig) (leaderElectionDurations, error) {
	leaseDuration, err := parseDuration("leader election lease duration", cfg.LeaseDuration)
	if err != nil {
		return leaderElectionDurations{}, err
	}
	renewDeadline, err := parseDuration("leader election renew deadline", cfg.RenewDeadline)
	if err != nil {
		return leaderElectionDurations{}, err
	}
	retryPeriod, err := parseDuration("leader election retry period", cfg.RetryPeriod)
	if err != nil {
		return leaderElectionDurations{}, err
	}
	return leaderElectionDurations{
		leaseDuration: leaseDuration,
		renewDeadline: renewDeadline,
		retryPeriod:   retryPeriod,
	}, nil
}

func parseDuration(name, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s %q: %w", name, value, err)
	}
	return duration, nil
}

func leaderElectionIdentity(cfg config.LeaderElectionConfig) (string, error) {
	if cfg.Identity != "" {
		return cfg.Identity, nil
	}
	if podName := os.Getenv("POD_NAME"); podName != "" {
		return podName, nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve leader election identity: %w", err)
	}
	if hostname == "" {
		return "", fmt.Errorf("resolve leader election identity: hostname is empty")
	}
	return hostname, nil
}

func leaderElectionNamespace(cfg config.LeaderElectionConfig) string {
	if cfg.Namespace != "" {
		return cfg.Namespace
	}
	if podNamespace := os.Getenv("POD_NAMESPACE"); podNamespace != "" {
		return podNamespace
	}
	return "default"
}
