package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/sundi0331/logbook/config"
)

type staticEventWatcher struct {
	list *eventsv1.EventList
}

func (s staticEventWatcher) List(context.Context, metav1.ListOptions) (*eventsv1.EventList, error) {
	return s.list, nil
}

func (staticEventWatcher) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return watch.NewFake(), nil
}

type memoryCheckpointStore struct {
	resourceVersion string
	saveCount       int
}

func (s *memoryCheckpointStore) Load(context.Context) (string, error) {
	return s.resourceVersion, nil
}

func (s *memoryCheckpointStore) Save(_ context.Context, resourceVersion string) error {
	s.resourceVersion = resourceVersion
	s.saveCount++
	return nil
}

func (s *memoryCheckpointStore) Flush(context.Context) error {
	return nil
}

func TestCurrentResourceVersionReturnsListVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	events := staticEventWatcher{
		list: &eventsv1.EventList{
			ListMeta: metav1.ListMeta{ResourceVersion: "123"},
			Items: []eventsv1.Event{
				{ObjectMeta: metav1.ObjectMeta{Name: "existing-event"}},
			},
		},
	}

	resourceVersion, err := currentResourceVersion(context.Background(), events, metav1.ListOptions{}, logger)
	if err != nil {
		t.Fatalf("currentResourceVersion() error = %v", err)
	}
	if resourceVersion != "123" {
		t.Fatalf("currentResourceVersion() = %q, want 123", resourceVersion)
	}
}

func TestConsumeEventsLogsObservedEvent(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	watcher := watch.NewFake()

	go func() {
		watcher.Add(&eventsv1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "smoke-event",
				Namespace:       "default",
				ResourceVersion: "456",
			},
			Reason: "SmokeTest",
		})
		watcher.Stop()
	}()

	checkpoint := &memoryCheckpointStore{}
	resourceVersion, err := consumeEvents(context.Background(), watcher, checkpoint, 0, logger)
	if err != nil {
		t.Fatalf("consumeEvents() error = %v", err)
	}
	if resourceVersion != "456" {
		t.Fatalf("consumeEvents() resourceVersion = %q, want 456", resourceVersion)
	}
	if checkpoint.resourceVersion != "456" {
		t.Fatalf("checkpoint resourceVersion = %q, want 456", checkpoint.resourceVersion)
	}

	logged := output.String()
	if !strings.Contains(logged, "kubernetes event observed") {
		t.Fatalf("consumeEvents() log = %q, want observed event message", logged)
	}
	if !strings.Contains(logged, "SmokeTest") {
		t.Fatalf("consumeEvents() log = %q, want event reason", logged)
	}
}

func TestConsumeEventsReturnsExpiredResourceVersionError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	watcher := watch.NewFake()

	go func() {
		watcher.Error(&metav1.Status{
			Status: metav1.StatusFailure,
			Reason: metav1.StatusReasonExpired,
			Code:   http.StatusGone,
		})
		watcher.Stop()
	}()

	_, err := consumeEvents(context.Background(), watcher, nil, 0, logger)
	if !errors.Is(err, errResourceVersionExpired) {
		t.Fatalf("consumeEvents() error = %v, want errResourceVersionExpired", err)
	}
}

func TestRecoverExpiredResourceVersionSkipsExistingAndSavesCheckpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	events := staticEventWatcher{
		list: &eventsv1.EventList{
			ListMeta: metav1.ListMeta{ResourceVersion: "789"},
		},
	}
	checkpoint := &memoryCheckpointStore{}
	cfg := &config.Config{
		Checkpoint: config.CheckpointConfig{OnExpiredResourceVersion: "skip-existing"},
	}

	resourceVersion, err := recoverExpiredResourceVersion(context.Background(), cfg, events, checkpoint, logger)
	if err != nil {
		t.Fatalf("recoverExpiredResourceVersion() error = %v", err)
	}
	if resourceVersion != "789" {
		t.Fatalf("recoverExpiredResourceVersion() = %q, want 789", resourceVersion)
	}
	if checkpoint.resourceVersion != "789" {
		t.Fatalf("checkpoint resourceVersion = %q, want 789", checkpoint.resourceVersion)
	}
}

func TestRecoverExpiredResourceVersionFailPolicy(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	cfg := &config.Config{
		Checkpoint: config.CheckpointConfig{OnExpiredResourceVersion: "fail"},
	}

	_, err := recoverExpiredResourceVersion(context.Background(), cfg, staticEventWatcher{}, nil, logger)
	if !errors.Is(err, errResourceVersionExpired) {
		t.Fatalf("recoverExpiredResourceVersion() error = %v, want errResourceVersionExpired", err)
	}
}

func TestConsumeEventsReturnsNonExpiredWatchError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	watcher := watch.NewFake()

	go func() {
		watcher.Error(&metav1.Status{
			Status: metav1.StatusFailure,
			Reason: metav1.StatusReasonNotFound,
			Code:   http.StatusNotFound,
		})
		watcher.Stop()
	}()

	_, err := consumeEvents(context.Background(), watcher, nil, 0, logger)
	if err == nil {
		t.Fatal("consumeEvents() error = nil, want error")
	}
	if errors.Is(err, errResourceVersionExpired) {
		t.Fatalf("consumeEvents() error = %v, did not want errResourceVersionExpired", err)
	}
}

func TestBufferedCheckpointStoreCoalescesSavesUntilFlush(t *testing.T) {
	base := &memoryCheckpointStore{}
	checkpoint := newBufferedCheckpointStore(base)

	if err := checkpoint.Save(context.Background(), "100"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := checkpoint.Save(context.Background(), "101"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if base.saveCount != 0 {
		t.Fatalf("base saveCount = %d, want 0 before flush", base.saveCount)
	}

	if err := checkpoint.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if base.resourceVersion != "101" {
		t.Fatalf("base resourceVersion = %q, want 101", base.resourceVersion)
	}
	if base.saveCount != 1 {
		t.Fatalf("base saveCount = %d, want 1", base.saveCount)
	}

	if err := checkpoint.Flush(context.Background()); err != nil {
		t.Fatalf("second Flush() error = %v", err)
	}
	if base.saveCount != 1 {
		t.Fatalf("base saveCount after second flush = %d, want 1", base.saveCount)
	}
}

func TestConsumeEventsFlushesBufferedCheckpointOnClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	base := &memoryCheckpointStore{}
	checkpoint := newBufferedCheckpointStore(base)
	watcher := watch.NewFake()

	go func() {
		watcher.Add(&eventsv1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "checkpoint-event",
				ResourceVersion: "900",
			},
		})
		watcher.Stop()
	}()

	resourceVersion, err := consumeEvents(context.Background(), watcher, checkpoint, time.Hour, logger)
	if err != nil {
		t.Fatalf("consumeEvents() error = %v", err)
	}
	if resourceVersion != "900" {
		t.Fatalf("consumeEvents() resourceVersion = %q, want 900", resourceVersion)
	}
	if base.resourceVersion != "900" {
		t.Fatalf("base resourceVersion = %q, want 900", base.resourceVersion)
	}
	if base.saveCount != 1 {
		t.Fatalf("base saveCount = %d, want 1", base.saveCount)
	}
}

func TestCheckpointDisabledIgnoresInvalidFlushInterval(t *testing.T) {
	store, err := newCheckpointStore(config.CheckpointConfig{
		Enabled:       false,
		FlushInterval: "not-a-duration",
	}, nil)
	if err != nil {
		t.Fatalf("newCheckpointStore() error = %v", err)
	}
	if store != nil {
		t.Fatalf("newCheckpointStore() = %T, want nil", store)
	}
}

func TestFileCheckpointStoreLoadTrimsWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint")
	if err := os.WriteFile(path, []byte("12345\n"), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resourceVersion, err := newFileCheckpointStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if resourceVersion != "12345" {
		t.Fatalf("Load() = %q, want 12345", resourceVersion)
	}
}

func TestConfigMapCheckpointStoreCreatesBeforePatchWhenMissing(t *testing.T) {
	client := fake.NewSimpleClientset()
	store := newConfigMapCheckpointStore(client.CoreV1().ConfigMaps("default"), "logbook-checkpoint")

	if err := store.Save(context.Background(), "123"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	actions := client.Actions()
	if len(actions) != 1 {
		t.Fatalf("actions = %#v, want exactly one create action", actions)
	}
	if action := actions[0].GetVerb(); action != "create" {
		t.Fatalf("first action = %q, want create", action)
	}
}

func TestConfigMapCheckpointStorePatchesAfterLoad(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "logbook-checkpoint", Namespace: "default"},
		Data:       map[string]string{checkpointResourceVersionKey: "100"},
	})
	store := newConfigMapCheckpointStore(client.CoreV1().ConfigMaps("default"), "logbook-checkpoint")

	if _, err := store.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	client.ClearActions()

	if err := store.Save(context.Background(), "101"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	actions := client.Actions()
	if len(actions) != 1 {
		t.Fatalf("actions = %#v, want exactly one patch action", actions)
	}
	if action := actions[0].GetVerb(); action != "patch" {
		t.Fatalf("first action = %q, want patch", action)
	}
}

func TestLeaderElectionDisabledIgnoresInvalidTiming(t *testing.T) {
	called := false
	err := runWithLeaderElection(context.Background(), config.LeaderElectionConfig{
		Enabled:       false,
		LeaseDuration: "not-a-duration",
	}, leaderElectionClients{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("runWithLeaderElection() error = %v", err)
	}
	if !called {
		t.Fatal("runWithLeaderElection() did not call run function")
	}
}

func TestLeaderElectionTimingParsesDurations(t *testing.T) {
	timing, err := leaderElectionTiming(config.LeaderElectionConfig{
		LeaseDuration: "15s",
		RenewDeadline: "10s",
		RetryPeriod:   "2s",
	})
	if err != nil {
		t.Fatalf("leaderElectionTiming() error = %v", err)
	}
	if timing.leaseDuration != 15*time.Second {
		t.Fatalf("leaseDuration = %s, want 15s", timing.leaseDuration)
	}
	if timing.renewDeadline != 10*time.Second {
		t.Fatalf("renewDeadline = %s, want 10s", timing.renewDeadline)
	}
	if timing.retryPeriod != 2*time.Second {
		t.Fatalf("retryPeriod = %s, want 2s", timing.retryPeriod)
	}
}

func TestLeaderElectionIdentityUsesConfiguredIdentity(t *testing.T) {
	identity, err := leaderElectionIdentity(config.LeaderElectionConfig{Identity: "configured"})
	if err != nil {
		t.Fatalf("leaderElectionIdentity() error = %v", err)
	}
	if identity != "configured" {
		t.Fatalf("leaderElectionIdentity() = %q, want configured", identity)
	}
}

func TestLeaderElectionIdentityUsesPodName(t *testing.T) {
	t.Setenv("POD_NAME", "pod-identity")

	identity, err := leaderElectionIdentity(config.LeaderElectionConfig{})
	if err != nil {
		t.Fatalf("leaderElectionIdentity() error = %v", err)
	}
	if identity != "pod-identity" {
		t.Fatalf("leaderElectionIdentity() = %q, want pod-identity", identity)
	}
}

func TestLeaderElectionNamespaceFallback(t *testing.T) {
	if namespace := leaderElectionNamespace(config.LeaderElectionConfig{Namespace: "configured"}); namespace != "configured" {
		t.Fatalf("configured namespace = %q, want configured", namespace)
	}

	t.Setenv("POD_NAMESPACE", "pod-namespace")
	if namespace := leaderElectionNamespace(config.LeaderElectionConfig{}); namespace != "pod-namespace" {
		t.Fatalf("POD_NAMESPACE namespace = %q, want pod-namespace", namespace)
	}

	t.Setenv("POD_NAMESPACE", "")
	if namespace := leaderElectionNamespace(config.LeaderElectionConfig{}); namespace != "default" {
		t.Fatalf("default namespace = %q, want default", namespace)
	}
}

func TestLeaderElectionEnabledValidatesTimingBeforeRunning(t *testing.T) {
	err := runWithLeaderElection(context.Background(), config.LeaderElectionConfig{
		Enabled:       true,
		Name:          "logbook-leader",
		Identity:      "test-identity",
		LeaseDuration: "invalid",
		RenewDeadline: "10s",
		RetryPeriod:   "2s",
	}, leaderElectionClients{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func(context.Context) error {
		t.Fatal("run function should not be called with invalid leader election timing")
		return nil
	})
	if err == nil {
		t.Fatal("runWithLeaderElection() error = nil, want error")
	}
}

func TestLeaderElectionWaitsForRunCallbackAfterContextCancel(t *testing.T) {
	client := fake.NewSimpleClientset()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	finished := make(chan struct{})
	returned := make(chan error, 1)

	go func() {
		returned <- runWithLeaderElection(ctx, config.LeaderElectionConfig{
			Enabled:       true,
			Namespace:     "default",
			Name:          "logbook-leader",
			Identity:      "test-identity",
			LeaseDuration: "2s",
			RenewDeadline: "1500ms",
			RetryPeriod:   "100ms",
		}, leaderElectionClients{
			core:         client.CoreV1(),
			coordination: client.CoordinationV1(),
		}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func(runCtx context.Context) error {
			close(started)
			cancel()
			<-runCtx.Done()
			time.Sleep(100 * time.Millisecond)
			close(finished)
			return nil
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("leader election run callback did not start")
	}

	select {
	case err := <-returned:
		t.Fatalf("runWithLeaderElection() returned before callback finished: %v", err)
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("leader election run callback did not finish")
	}

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("runWithLeaderElection() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWithLeaderElection() did not return after callback finished")
	}
}

func TestLeaderElectionStopsWhenRunCallbackReturnsNil(t *testing.T) {
	client := fake.NewSimpleClientset()
	returned := make(chan error, 1)

	go func() {
		returned <- runWithLeaderElection(context.Background(), config.LeaderElectionConfig{
			Enabled:       true,
			Namespace:     "default",
			Name:          "logbook-leader",
			Identity:      "test-identity",
			LeaseDuration: "2s",
			RenewDeadline: "1500ms",
			RetryPeriod:   "100ms",
		}, leaderElectionClients{
			core:         client.CoreV1(),
			coordination: client.CoordinationV1(),
		}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func(context.Context) error {
			return nil
		})
	}()

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("runWithLeaderElection() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWithLeaderElection() did not return after callback returned nil")
	}
}
