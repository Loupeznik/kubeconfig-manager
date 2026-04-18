package state

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	return NewFileStore(filepath.Join(t.TempDir(), "config.yaml"))
}

func TestLoadMissingReturnsEmptyConfig(t *testing.T) {
	s := newTestStore(t)
	cfg, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != CurrentVersion {
		t.Errorf("version: got %d, want %d", cfg.Version, CurrentVersion)
	}
	if len(cfg.Entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(cfg.Entries))
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	cfg := NewConfig()
	cfg.KubeconfigDir = "/tmp/kube"
	cfg.Entries["sha256:abc"] = Entry{
		PathHint:    "prod.yaml",
		DisplayName: "Prod",
		Tags:        []string{"prod", "eu"},
		Alerts: Alerts{
			Enabled:             true,
			RequireConfirmation: true,
			BlockedVerbs:        []string{"delete", "drain"},
		},
		UpdatedAt: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC),
	}
	if err := s.Save(ctx, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.KubeconfigDir != "/tmp/kube" {
		t.Errorf("dir: got %q", got.KubeconfigDir)
	}
	entry, ok := got.Entries["sha256:abc"]
	if !ok {
		t.Fatal("entry missing after round-trip")
	}
	if entry.DisplayName != "Prod" {
		t.Errorf("display_name: got %q", entry.DisplayName)
	}
	if len(entry.Tags) != 2 || entry.Tags[0] != "prod" {
		t.Errorf("tags: got %v", entry.Tags)
	}
	if !entry.Alerts.Enabled {
		t.Error("alerts.enabled lost after round-trip")
	}
	if !entry.UpdatedAt.Equal(time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("updated_at: got %v", entry.UpdatedAt)
	}
}

func TestSaveHasSecureFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits do not apply on Windows")
	}
	ctx := context.Background()
	s := newTestStore(t)
	cfg := NewConfig()
	if err := s.Save(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode: got %o, want 0o600", got)
	}
}

func TestMutateRebindsOnRename(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	hash := "sha256:stable"

	err := s.Mutate(ctx, func(cfg *Config) error {
		entry := cfg.Entries[hash]
		entry.PathHint = "prod.yaml"
		entry.Tags = []string{"prod"}
		entry.Touch()
		cfg.Entries[hash] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = s.Mutate(ctx, func(cfg *Config) error {
		entry := cfg.Entries[hash]
		entry.PathHint = "prod-eu.yaml"
		entry.Touch()
		cfg.Entries[hash] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry := cfg.Entries[hash]
	if entry.PathHint != "prod-eu.yaml" {
		t.Errorf("path_hint: got %q, want prod-eu.yaml", entry.PathHint)
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "prod" {
		t.Errorf("tags lost after rename: got %v", entry.Tags)
	}
}

func TestAddTagsIsIdempotent(t *testing.T) {
	e := Entry{}
	added := e.AddTags("prod", "eu", "prod")
	if len(added) != 2 {
		t.Errorf("added: got %v, want 2 unique", added)
	}
	if len(e.Tags) != 2 {
		t.Errorf("tags: got %v, want 2 unique", e.Tags)
	}
	more := e.AddTags("prod")
	if len(more) != 0 {
		t.Errorf("second add: got %v, want empty", more)
	}
}

func TestRemoveTags(t *testing.T) {
	e := Entry{Tags: []string{"prod", "eu", "critical"}}
	removed := e.RemoveTags("eu", "missing")
	if len(removed) != 1 || removed[0] != "eu" {
		t.Errorf("removed: got %v", removed)
	}
	if len(e.Tags) != 2 {
		t.Errorf("tags: got %v, want 2 remaining", e.Tags)
	}
}

func TestEnsurePaletteMergesMissingEntryTags(t *testing.T) {
	cfg := NewConfig()
	cfg.AvailableTags = []string{"test", "xd"}
	cfg.Entries["sha256:abc"] = Entry{
		Tags: []string{"orig", "test"},
		ContextTags: map[string][]string{
			"prod-eu": {"ctx-only"},
		},
	}

	added := cfg.EnsurePaletteFromEntries()

	wantAdded := map[string]bool{"orig": true, "ctx-only": true}
	if len(added) != 2 {
		t.Errorf("added: got %v, want orig + ctx-only", added)
	}
	for _, a := range added {
		if !wantAdded[a] {
			t.Errorf("unexpected tag added: %q", a)
		}
	}

	// Existing palette order preserved; new tags appended.
	if got := cfg.AvailableTags[:2]; got[0] != "test" || got[1] != "xd" {
		t.Errorf("palette head reordered: %v", got)
	}
	seen := map[string]bool{}
	for _, t := range cfg.AvailableTags {
		seen[t] = true
	}
	for _, want := range []string{"test", "xd", "orig", "ctx-only"} {
		if !seen[want] {
			t.Errorf("palette missing %q after merge: %v", want, cfg.AvailableTags)
		}
	}
}

func TestEnsurePaletteIdempotent(t *testing.T) {
	cfg := NewConfig()
	cfg.AvailableTags = []string{"prod", "staging"}
	cfg.Entries["sha256:abc"] = Entry{Tags: []string{"prod"}}

	first := cfg.EnsurePaletteFromEntries()
	second := cfg.EnsurePaletteFromEntries()
	if len(first) != 0 || len(second) != 0 {
		t.Errorf("unexpected additions: first=%v second=%v", first, second)
	}
	if len(cfg.AvailableTags) != 2 {
		t.Errorf("palette grew: %v", cfg.AvailableTags)
	}
}

func TestMigrateRejectsFutureVersion(t *testing.T) {
	cfg := &Config{Version: 99}
	if err := migrate(cfg); err == nil {
		t.Fatal("expected error for future version")
	}
}

func TestConcurrentMutatesAreSerializedByLock(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		hash := "sha256:" + string(rune('a'+i))
		go func(h string) {
			defer wg.Done()
			_ = s.Mutate(ctx, func(cfg *Config) error {
				entry := cfg.Entries[h]
				entry.Tags = append(entry.Tags, "concurrent")
				entry.Touch()
				cfg.Entries[h] = entry
				return nil
			})
		}(hash)
	}
	wg.Wait()

	cfg, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Entries) != n {
		t.Errorf("entries: got %d, want %d (lock may have failed)", len(cfg.Entries), n)
	}
}
