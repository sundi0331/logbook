package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
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

	resourceVersion, err := consumeEvents(context.Background(), watcher, logger)
	if err != nil {
		t.Fatalf("consumeEvents() error = %v", err)
	}
	if resourceVersion != "456" {
		t.Fatalf("consumeEvents() resourceVersion = %q, want 456", resourceVersion)
	}

	logged := output.String()
	if !strings.Contains(logged, "kubernetes event observed") {
		t.Fatalf("consumeEvents() log = %q, want observed event message", logged)
	}
	if !strings.Contains(logged, "SmokeTest") {
		t.Fatalf("consumeEvents() log = %q, want event reason", logged)
	}
}
