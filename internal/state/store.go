package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"github.com/gofrs/flock"
	"gopkg.in/yaml.v3"
)

const (
	appDir     = "kubeconfig-manager"
	configFile = "config.yaml"
)

// Store is the persistence interface for a Config. FileStore is the v0.9.x
// implementation; cloud-sync backends (git, S3, Vault) will plug in without
// changing the CLI or TUI.
type Store interface {
	Load(ctx context.Context) (*Config, error)
	Save(ctx context.Context, cfg *Config) error
	Mutate(ctx context.Context, fn func(cfg *Config) error) error
	Path() string
}

// FileStore persists Config as YAML at a local path, with atomic writes
// (temp file + rename) and a flock-based Mutate for cross-process safety.
type FileStore struct {
	path     string
	lockPath string
}

// NewFileStore constructs a FileStore whose state file is at path. The lock
// file is path + ".lock".
func NewFileStore(path string) *FileStore {
	return &FileStore{
		path:     path,
		lockPath: path + ".lock",
	}
}

// DefaultStore returns a FileStore at DefaultPath().
func DefaultStore() (*FileStore, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return NewFileStore(path), nil
}

// DefaultPath resolves the canonical state-file location under XDG_CONFIG_HOME.
func DefaultPath() (string, error) {
	p, err := xdg.ConfigFile(filepath.Join(appDir, configFile))
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	return p, nil
}

// Path returns the absolute path of the state file.
func (s *FileStore) Path() string { return s.path }

// Load reads and parses the state file. A missing file returns a fresh
// NewConfig() rather than an error so first-run flows work without an explicit
// init step.
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

// Save writes cfg atomically. The temp file is created in the same directory
// as the final file so the rename stays on one filesystem.
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

// Mutate serializes a read-modify-write cycle under an exclusive flock on
// path + ".lock". The callback runs against a freshly-loaded Config; returning
// an error aborts the write.
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

// migrate bumps an older schema version in-place. Today no migration steps
// exist — the function just verifies the version is supported.
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
