package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/sundi0331/logbook/config"
)

const checkpointResourceVersionKey = "resourceVersion"

type checkpointStore interface {
	Load(ctx context.Context) (string, error)
	Save(ctx context.Context, resourceVersion string) error
	Flush(ctx context.Context) error
}

type fileCheckpointStore struct {
	path string
}

func newFileCheckpointStore(path string) *fileCheckpointStore {
	return &fileCheckpointStore{path: path}
}

func (s *fileCheckpointStore) Load(context.Context) (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read checkpoint file %q: %w", s.path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *fileCheckpointStore) Save(_ context.Context, resourceVersion string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("create checkpoint directory %q: %w", filepath.Dir(s.path), err)
	}

	tmp := fmt.Sprintf("%s.%d.tmp", s.path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte(resourceVersion), 0o640); err != nil {
		return fmt.Errorf("write checkpoint file %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace checkpoint file %q: %w", s.path, err)
	}
	return nil
}

func (s *fileCheckpointStore) Flush(context.Context) error {
	return nil
}

type configMapCheckpointStore struct {
	configMaps coreclientv1.ConfigMapInterface
	name       string
	mu         sync.Mutex
	exists     bool
}

func newConfigMapCheckpointStore(configMaps coreclientv1.ConfigMapInterface, name string) *configMapCheckpointStore {
	return &configMapCheckpointStore{configMaps: configMaps, name: name}
}

func (s *configMapCheckpointStore) Load(ctx context.Context) (string, error) {
	checkpoint, err := s.configMaps.Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.setExists(false)
			return "", nil
		}
		return "", fmt.Errorf("get checkpoint ConfigMap %q: %w", s.name, err)
	}
	s.setExists(true)
	return checkpoint.Data[checkpointResourceVersionKey], nil
}

func (s *configMapCheckpointStore) Save(ctx context.Context, resourceVersion string) error {
	exists := s.checkpointExists()
	for {
		if !exists {
			checkpoint := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: s.name},
				Data: map[string]string{
					checkpointResourceVersionKey: resourceVersion,
				},
			}
			if _, err := s.configMaps.Create(ctx, checkpoint, metav1.CreateOptions{}); err != nil {
				if apierrors.IsAlreadyExists(err) {
					s.setExists(true)
					exists = true
					continue
				}
				return fmt.Errorf("create checkpoint ConfigMap %q: %w", s.name, err)
			}
			s.setExists(true)
			return nil
		}

		patch, err := json.Marshal(map[string]map[string]string{
			"data": {
				checkpointResourceVersionKey: resourceVersion,
			},
		})
		if err != nil {
			return fmt.Errorf("marshal checkpoint ConfigMap patch: %w", err)
		}
		_, err = s.configMaps.Patch(ctx, s.name, types.MergePatchType, patch, metav1.PatchOptions{})
		if err == nil {
			s.setExists(true)
			return nil
		}
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("patch checkpoint ConfigMap %q: %w", s.name, err)
		}
		s.setExists(false)
		exists = false
	}
}

func (s *configMapCheckpointStore) Flush(context.Context) error {
	return nil
}

func (s *configMapCheckpointStore) checkpointExists() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exists
}

func (s *configMapCheckpointStore) setExists(exists bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exists = exists
}

type bufferedCheckpointStore struct {
	store checkpointStore

	mu      sync.Mutex
	latest  string
	flushed string
	dirty   bool
}

func newBufferedCheckpointStore(store checkpointStore) *bufferedCheckpointStore {
	return &bufferedCheckpointStore{store: store}
}

func (s *bufferedCheckpointStore) Load(ctx context.Context) (string, error) {
	resourceVersion, err := s.store.Load(ctx)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.latest = resourceVersion
	s.flushed = resourceVersion
	s.dirty = false
	s.mu.Unlock()
	return resourceVersion, nil
}

func (s *bufferedCheckpointStore) Save(_ context.Context, resourceVersion string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.latest = resourceVersion
	s.dirty = resourceVersion != s.flushed
	return nil
}

func (s *bufferedCheckpointStore) Flush(ctx context.Context) error {
	s.mu.Lock()
	if !s.dirty || s.latest == "" {
		s.mu.Unlock()
		return nil
	}
	resourceVersion := s.latest
	s.mu.Unlock()

	if err := s.store.Save(ctx, resourceVersion); err != nil {
		return err
	}

	s.mu.Lock()
	if s.latest == resourceVersion {
		s.flushed = resourceVersion
		s.dirty = false
	}
	s.mu.Unlock()
	return nil
}

func newCheckpointStore(cfg config.CheckpointConfig, coreClient *coreclientv1.CoreV1Client) (checkpointStore, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	var store checkpointStore
	switch cfg.Backend {
	case "configmap":
		if coreClient == nil {
			return nil, fmt.Errorf("create configmap checkpoint store: Kubernetes core client is required")
		}
		namespace := cfg.Namespace
		if namespace == "" {
			namespace = os.Getenv("POD_NAMESPACE")
		}
		if namespace == "" {
			namespace = "default"
		}
		store = newConfigMapCheckpointStore(coreClient.ConfigMaps(namespace), cfg.Name)
	case "file":
		store = newFileCheckpointStore(cfg.Path)
	default:
		return nil, fmt.Errorf("unsupported checkpoint backend %q: expected configmap or file", cfg.Backend)
	}

	flushInterval, err := checkpointFlushInterval(cfg)
	if err != nil {
		return nil, err
	}
	if flushInterval <= 0 {
		return store, nil
	}
	return newBufferedCheckpointStore(store), nil
}

func checkpointFlushInterval(cfg config.CheckpointConfig) (time.Duration, error) {
	if cfg.FlushInterval == "" {
		return 0, nil
	}
	flushInterval, err := time.ParseDuration(cfg.FlushInterval)
	if err != nil {
		return 0, fmt.Errorf("parse checkpoint flush interval %q: %w", cfg.FlushInterval, err)
	}
	return flushInterval, nil
}
