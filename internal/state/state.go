package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/gofrs/flock"
	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion = 1
	appDir         = "kubeconfig-manager"
	configFile     = "config.yaml"
)

type Entry struct {
	PathHint    string    `yaml:"path_hint,omitempty"`
	DisplayName string    `yaml:"display_name,omitempty"`
	Tags        []string  `yaml:"tags,omitempty"`
	Alerts      Alerts    `yaml:"alerts,omitempty"`
	UpdatedAt   time.Time `yaml:"updated_at"`
}

type Alerts struct {
	Enabled             bool     `yaml:"enabled"`
	RequireConfirmation bool     `yaml:"require_confirmation,omitempty"`
	ConfirmClusterName  bool     `yaml:"confirm_cluster_name,omitempty"`
	BlockedVerbs        []string `yaml:"blocked_verbs,omitempty"`
}

type Config struct {
	Version       int              `yaml:"version"`
	KubeconfigDir string           `yaml:"kubeconfig_dir,omitempty"`
	Entries       map[string]Entry `yaml:"entries,omitempty"`
}

func NewConfig() *Config {
	return &Config{
		Version: CurrentVersion,
		Entries: map[string]Entry{},
	}
}

func DefaultBlockedVerbs() []string {
	return []string{"delete", "drain", "cordon", "uncordon", "taint", "replace", "patch"}
}

type Store interface {
	Load(ctx context.Context) (*Config, error)
	Save(ctx context.Context, cfg *Config) error
	Mutate(ctx context.Context, fn func(cfg *Config) error) error
	Path() string
}

type FileStore struct {
	path     string
	lockPath string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{
		path:     path,
		lockPath: path + ".lock",
	}
}

func DefaultStore() (*FileStore, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return NewFileStore(path), nil
}

func DefaultPath() (string, error) {
	p, err := xdg.ConfigFile(filepath.Join(appDir, configFile))
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	return p, nil
}

func (s *FileStore) Path() string { return s.path }

func (s *FileStore) Load(_ context.Context) (*Config, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return NewConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	cfg := NewConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if cfg.Entries == nil {
		cfg.Entries = map[string]Entry{}
	}
	if err := migrate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *FileStore) Save(_ context.Context, cfg *Config) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp state: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename temp state: %w", err)
	}
	return nil
}

func (s *FileStore) Mutate(ctx context.Context, fn func(cfg *Config) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	lock := flock.New(s.lockPath)
	locked, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire state lock: %w", err)
	}
	if !locked {
		return errors.New("could not acquire state lock")
	}
	defer func() {
		_ = lock.Unlock()
	}()

	cfg, err := s.Load(ctx)
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		return err
	}
	return s.Save(ctx, cfg)
}

func (e *Entry) Touch() {
	e.UpdatedAt = time.Now().UTC()
}

func (e *Entry) AddTags(tags ...string) (added []string) {
	seen := map[string]bool{}
	for _, t := range e.Tags {
		seen[t] = true
	}
	for _, t := range tags {
		t = normalizeTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		e.Tags = append(e.Tags, t)
		added = append(added, t)
	}
	return added
}

func (e *Entry) RemoveTags(tags ...string) (removed []string) {
	drop := map[string]bool{}
	for _, t := range tags {
		drop[normalizeTag(t)] = true
	}
	kept := e.Tags[:0]
	for _, t := range e.Tags {
		if drop[t] {
			removed = append(removed, t)
			continue
		}
		kept = append(kept, t)
	}
	e.Tags = kept
	return removed
}

func normalizeTag(t string) string {
	return strings.TrimSpace(t)
}

func migrate(cfg *Config) error {
	switch cfg.Version {
	case 0:
		cfg.Version = CurrentVersion
	case CurrentVersion:
	default:
		return fmt.Errorf("unsupported state version %d (this build supports up to %d)", cfg.Version, CurrentVersion)
	}
	return nil
}
