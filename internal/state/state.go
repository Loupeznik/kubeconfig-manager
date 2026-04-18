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
	PathHint      string              `yaml:"path_hint,omitempty"`
	DisplayName   string              `yaml:"display_name,omitempty"`
	Tags          []string            `yaml:"tags,omitempty"`
	Alerts        Alerts              `yaml:"alerts,omitempty"`
	ContextAlerts map[string]Alerts   `yaml:"context_alerts,omitempty"`
	ContextTags   map[string][]string `yaml:"context_tags,omitempty"`
	HelmGuard     *HelmGuard          `yaml:"helm_guard,omitempty"`
	UpdatedAt     time.Time           `yaml:"updated_at"`
}

type Alerts struct {
	Enabled             bool     `yaml:"enabled"`
	RequireConfirmation bool     `yaml:"require_confirmation,omitempty"`
	ConfirmClusterName  bool     `yaml:"confirm_cluster_name,omitempty"`
	BlockedVerbs        []string `yaml:"blocked_verbs,omitempty"`
}

// HelmGuard configures the helm values-path ↔ context mismatch detector.
// Applied at two scopes with per-entry overriding global (see ResolveHelmGuard).
type HelmGuard struct {
	Enabled bool `yaml:"enabled"`
	// Pattern is the values-file path template used to extract a name, e.g.
	// "clusters/{name}/" matches ".../clusters/prod-eu/values.yaml" and
	// produces "prod-eu". When empty, falls back to the global pattern (or
	// DefaultHelmPattern if global is also empty).
	Pattern string `yaml:"pattern,omitempty"`
	// EnvTokens is the set of "environment-like" tokens that, if they appear
	// on one side of the comparison but not the other, mark a high-severity
	// mismatch. When empty, falls back to global then DefaultEnvTokens.
	EnvTokens []string `yaml:"env_tokens,omitempty"`
	// RequireConfirmation reads as true unless explicitly set to false.
	RequireConfirmation bool `yaml:"require_confirmation,omitempty"`
}

// DefaultHelmPattern is the fallback values-file path template.
const DefaultHelmPattern = "clusters/{name}/"

// DefaultEnvTokens returns the canonical environment-token set used when
// neither per-entry nor global HelmGuard sets one.
func DefaultEnvTokens() []string {
	return []string{
		"prod", "production",
		"staging", "stg", "stage",
		"dev", "development",
		"test", "tst",
		"qa",
		"sandbox", "sbx",
		"preprod", "preview",
	}
}

// ResolveHelmGuard returns the effective HelmGuard for this entry, falling
// back from per-entry to global. A per-entry nil means "inherit global";
// a per-entry struct with Enabled=false is an explicit override to suppress.
func (e Entry) ResolveHelmGuard(global HelmGuard) HelmGuard {
	base := global
	if e.HelmGuard == nil {
		if base.Pattern == "" {
			base.Pattern = DefaultHelmPattern
		}
		if len(base.EnvTokens) == 0 {
			base.EnvTokens = DefaultEnvTokens()
		}
		return base
	}
	out := *e.HelmGuard
	if out.Pattern == "" {
		out.Pattern = base.Pattern
	}
	if out.Pattern == "" {
		out.Pattern = DefaultHelmPattern
	}
	if len(out.EnvTokens) == 0 {
		out.EnvTokens = base.EnvTokens
	}
	if len(out.EnvTokens) == 0 {
		out.EnvTokens = DefaultEnvTokens()
	}
	return out
}

// ResolveAlerts returns the active alert policy for a given context within
// this entry. A per-context override (if present) wins over the file-level
// policy; if neither is set, the zero Alerts{} is returned.
func (e Entry) ResolveAlerts(contextName string) Alerts {
	if contextName != "" {
		if a, ok := e.ContextAlerts[contextName]; ok {
			return a
		}
	}
	return e.Alerts
}

// ResolveTags returns the effective tag set for a context — union of the
// file-level tags and the context-level tags, deduplicated and order-preserving
// (file-level first, then context-level additions).
func (e Entry) ResolveTags(contextName string) []string {
	seen := make(map[string]bool, len(e.Tags))
	out := make([]string, 0, len(e.Tags))
	for _, t := range e.Tags {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if contextName != "" {
		for _, t := range e.ContextTags[contextName] {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

// AddContextTags adds unique tags to the given context's tag list on the Entry.
func (e *Entry) AddContextTags(contextName string, tags ...string) (added []string) {
	if e.ContextTags == nil {
		e.ContextTags = map[string][]string{}
	}
	existing := e.ContextTags[contextName]
	seen := map[string]bool{}
	for _, t := range existing {
		seen[t] = true
	}
	for _, t := range tags {
		t = normalizeTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		existing = append(existing, t)
		added = append(added, t)
	}
	e.ContextTags[contextName] = existing
	return added
}

// RemoveContextTags removes tags from the given context's tag list.
func (e *Entry) RemoveContextTags(contextName string, tags ...string) (removed []string) {
	if e.ContextTags == nil {
		return nil
	}
	drop := map[string]bool{}
	for _, t := range tags {
		drop[normalizeTag(t)] = true
	}
	existing := e.ContextTags[contextName]
	kept := existing[:0]
	for _, t := range existing {
		if drop[t] {
			removed = append(removed, t)
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		delete(e.ContextTags, contextName)
	} else {
		e.ContextTags[contextName] = kept
	}
	return removed
}

type Config struct {
	Version       int              `yaml:"version"`
	KubeconfigDir string           `yaml:"kubeconfig_dir,omitempty"`
	AvailableTags []string         `yaml:"available_tags,omitempty"`
	HelmGuard     HelmGuard        `yaml:"helm_guard,omitempty"`
	Entries       map[string]Entry `yaml:"entries,omitempty"`
}

func NewConfig() *Config {
	return &Config{
		Version: CurrentVersion,
		Entries: map[string]Entry{},
	}
}

// AddAvailableTags inserts unique tags into the palette (order-preserving).
func (c *Config) AddAvailableTags(tags ...string) (added []string) {
	seen := map[string]bool{}
	for _, t := range c.AvailableTags {
		seen[t] = true
	}
	for _, t := range tags {
		t = normalizeTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		c.AvailableTags = append(c.AvailableTags, t)
		added = append(added, t)
	}
	return added
}

// RemoveAvailableTags drops tags from the palette; also scrubs them from every
// entry's file-level and per-context tag lists so listings stay consistent.
func (c *Config) RemoveAvailableTags(tags ...string) (removed []string) {
	drop := map[string]bool{}
	for _, t := range tags {
		drop[normalizeTag(t)] = true
	}
	kept := c.AvailableTags[:0]
	for _, t := range c.AvailableTags {
		if drop[t] {
			removed = append(removed, t)
			continue
		}
		kept = append(kept, t)
	}
	c.AvailableTags = kept

	// Scrub from entries.
	for hash, entry := range c.Entries {
		fileKept := entry.Tags[:0]
		for _, t := range entry.Tags {
			if !drop[t] {
				fileKept = append(fileKept, t)
			}
		}
		entry.Tags = fileKept
		for ctxName, ctxTags := range entry.ContextTags {
			ctxKept := ctxTags[:0]
			for _, t := range ctxTags {
				if !drop[t] {
					ctxKept = append(ctxKept, t)
				}
			}
			if len(ctxKept) == 0 {
				delete(entry.ContextTags, ctxName)
			} else {
				entry.ContextTags[ctxName] = ctxKept
			}
		}
		c.Entries[hash] = entry
	}
	return removed
}

// GetEntry finds an entry by its stable key. If not found, falls back to
// the legacy (content-hash) key as a read-only migration aid. Returns the
// zero Entry and false if neither key is present. Does not mutate the map.
func (c *Config) GetEntry(stableKey, legacyKey string) (Entry, bool) {
	if e, ok := c.Entries[stableKey]; ok {
		return e, true
	}
	if legacyKey != "" && legacyKey != stableKey {
		if e, ok := c.Entries[legacyKey]; ok {
			return e, true
		}
	}
	return Entry{}, false
}

// TakeEntry returns the entry for the given file identity, transparently
// migrating legacy-keyed entries to the stable key. Call inside a Mutate
// callback before making changes; the caller is expected to write the
// returned entry back under stableKey.
func (c *Config) TakeEntry(stableKey, legacyKey string) Entry {
	if e, ok := c.Entries[stableKey]; ok {
		return e
	}
	if legacyKey != "" && legacyKey != stableKey {
		if e, ok := c.Entries[legacyKey]; ok {
			delete(c.Entries, legacyKey)
			return e
		}
	}
	return Entry{}
}

// RenameAvailableTag renames a palette tag and updates every entry that
// references the old name (both file-level and per-context). Returns an error
// if the new name is empty, already present, or the old name is not found.
func (c *Config) RenameAvailableTag(oldTag, newTag string) error {
	oldTag = normalizeTag(oldTag)
	newTag = normalizeTag(newTag)
	if newTag == "" {
		return errors.New("new tag name cannot be empty")
	}
	if oldTag == newTag {
		return nil
	}
	for _, t := range c.AvailableTags {
		if t == newTag {
			return fmt.Errorf("tag %q already exists in palette", newTag)
		}
	}
	idx := -1
	for i, t := range c.AvailableTags {
		if t == oldTag {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("tag %q not found in palette", oldTag)
	}
	c.AvailableTags[idx] = newTag

	for hash, entry := range c.Entries {
		for i, t := range entry.Tags {
			if t == oldTag {
				entry.Tags[i] = newTag
			}
		}
		for ctxName, ctxTags := range entry.ContextTags {
			for i, t := range ctxTags {
				if t == oldTag {
					ctxTags[i] = newTag
				}
			}
			entry.ContextTags[ctxName] = ctxTags
		}
		c.Entries[hash] = entry
	}
	return nil
}

// IsTagInPalette checks membership.
func (c *Config) IsTagInPalette(tag string) bool {
	tag = normalizeTag(tag)
	for _, t := range c.AvailableTags {
		if t == tag {
			return true
		}
	}
	return false
}

// EnsurePaletteFromEntries makes sure every tag attached to any entry (file
// level or per-context) is present in the palette. Adds any missing tag,
// preserves existing palette order, dedupes. Safe to call repeatedly — it is
// both a first-run bootstrap and a repair step for state modified by older
// versions, `--allow-new` flows, or direct edits that bypass the palette.
//
// Returns the list of tags newly added to the palette.
func (c *Config) EnsurePaletteFromEntries() (added []string) {
	inPalette := make(map[string]bool, len(c.AvailableTags))
	for _, t := range c.AvailableTags {
		inPalette[t] = true
	}
	addNew := func(t string) {
		if t == "" || inPalette[t] {
			return
		}
		inPalette[t] = true
		c.AvailableTags = append(c.AvailableTags, t)
		added = append(added, t)
	}
	for _, entry := range c.Entries {
		for _, t := range entry.Tags {
			addNew(t)
		}
		for _, ctxTags := range entry.ContextTags {
			for _, t := range ctxTags {
				addNew(t)
			}
		}
	}
	return added
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
