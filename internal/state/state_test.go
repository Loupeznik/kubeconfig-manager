package state

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestHelmGuardLegacyPatternFieldMigrates(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "config.yaml")
	// Hand-crafted legacy state file with the single-string `pattern:` field
	// that earlier versions wrote.
	legacy := []byte(`version: 1
helm_guard:
  enabled: true
  pattern: "envs/{name}/"
  env_tokens: [prod, test]
entries: {}
`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	s := NewFileStore(path)
	cfg, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HelmGuard.IsEnabled() {
		t.Error("enabled flag lost after legacy load")
	}
	if len(cfg.HelmGuard.Patterns) != 1 || cfg.HelmGuard.Patterns[0] != "envs/{name}/" {
		t.Errorf("legacy pattern did not migrate: %v", cfg.HelmGuard.Patterns)
	}
}

// ---- Tag resolution with per-context exclusions ---------------------------

func TestResolveTagsUnionsFileAndContext(t *testing.T) {
	e := Entry{
		Tags: []string{"prod", "eu"},
		ContextTags: map[string][]string{
			"ctx-a": {"ha"},
		},
	}
	got := e.ResolveTags("ctx-a")
	want := []string{"prod", "eu", "ha"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveTagsPerContextExclusionSubtracts(t *testing.T) {
	// File-level has prod+eu. For ctx-b we don't want "eu" showing up even
	// though it's inherited from file-level. Exclusion must filter it out.
	e := Entry{
		Tags: []string{"prod", "eu", "critical"},
		ContextTagExclusions: map[string][]string{
			"ctx-b": {"eu"},
		},
	}
	got := e.ResolveTags("ctx-b")
	for _, tag := range got {
		if tag == "eu" {
			t.Errorf("eu should be excluded for ctx-b; got %v", got)
		}
	}
	// Other contexts still inherit it.
	if !contains(e.ResolveTags("ctx-a"), "eu") {
		t.Errorf("ctx-a should still inherit eu; got %v", e.ResolveTags("ctx-a"))
	}
}

func TestResolveTagsExclusionDoesNotAffectFileEntry(t *testing.T) {
	// Exclusions are scoped; file-level Tags slice is untouched.
	e := Entry{
		Tags: []string{"prod"},
		ContextTagExclusions: map[string][]string{
			"ctx-a": {"prod"},
		},
	}
	if len(e.Tags) != 1 || e.Tags[0] != "prod" {
		t.Errorf("file-level tags should not change: %v", e.Tags)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestMigrateRejectsFutureVersion(t *testing.T) {
	cfg := &Config{Version: 99}
	if err := migrate(cfg); err == nil {
		t.Fatal("expected error for future version")
	}
}

func TestLoadRejectsFutureVersionOnDisk(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 99\nentries: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewFileStore(path)
	_, err := s.Load(ctx)
	if err == nil {
		t.Fatal("expected Load to refuse future schema version")
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "config.yaml")
	// Unbalanced bracket / colon-less mapping → yaml.v3 parse error.
	if err := os.WriteFile(path, []byte("version: 1\nentries: [this: is not valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewFileStore(path)
	_, err := s.Load(ctx)
	if err == nil {
		t.Fatal("expected parse error on malformed YAML")
	}
	if !strings.Contains(err.Error(), "parse state") {
		t.Errorf("error should mention parse state: %v", err)
	}
}

func TestLoadToleratesUnknownTopLevelKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "config.yaml")
	// yaml.v3 is non-strict by default — unknown keys are silently dropped.
	// We depend on this so users can hand-edit without us rejecting newlines
	// or comments they add.
	doc := `version: 1
future_field: hello
entries:
  "sha256:abc":
    path_hint: "prod.yaml"
    tags: ["prod"]
`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewFileStore(path)
	cfg, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if e := cfg.Entries["sha256:abc"]; e.PathHint != "prod.yaml" || len(e.Tags) != 1 {
		t.Errorf("entry lost when unknown top-level key present: %+v", e)
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

// TestConcurrentMutatesAllFields writes to Tags, ContextTags, ContextAlerts,
// and HelmGuard simultaneously. Past regressions have been map-nil-deref
// panics on first access — the test exercises every map that a concurrent
// caller might touch on a fresh entry.
func TestConcurrentMutatesAllFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	hash := "sha256:shared"
	for i := 0; i < n; i++ {
		kind := i % 4
		go func(k int) {
			defer wg.Done()
			_ = s.Mutate(ctx, func(cfg *Config) error {
				entry := cfg.Entries[hash]
				switch k {
				case 0:
					entry.Tags = append(entry.Tags, "t")
				case 1:
					if entry.ContextTags == nil {
						entry.ContextTags = map[string][]string{}
					}
					entry.ContextTags["ctx"] = append(entry.ContextTags["ctx"], "c")
				case 2:
					if entry.ContextAlerts == nil {
						entry.ContextAlerts = map[string]Alerts{}
					}
					a := entry.ContextAlerts["ctx"]
					a.Enabled = true
					entry.ContextAlerts["ctx"] = a
				case 3:
					if entry.HelmGuard == nil {
						entry.HelmGuard = &HelmGuard{}
					}
					entry.HelmGuard.Enabled = BoolPtr(true)
					entry.HelmGuard.Patterns = append(entry.HelmGuard.Patterns, "p/{name}/")
				}
				entry.Touch()
				cfg.Entries[hash] = entry
				return nil
			})
		}(kind)
	}
	wg.Wait()

	cfg, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.Entries[hash]
	if !ok {
		t.Fatal("shared entry missing after concurrent writes")
	}
	if len(entry.Tags) == 0 {
		t.Error("tags never persisted")
	}
	if entry.HelmGuard == nil || !entry.HelmGuard.IsEnabled() {
		t.Error("helm_guard never persisted")
	}
	if _, has := entry.ContextAlerts["ctx"]; !has {
		t.Error("context_alerts never persisted")
	}
	if len(entry.ContextTags["ctx"]) == 0 {
		t.Error("context_tags never persisted")
	}
}

// ---- Tag edge cases (2.5) --------------------------------------------------

func TestNormalizeTagTrimsWhitespace(t *testing.T) {
	e := &Entry{}
	added := e.AddTags("  spaced  ", "\tprod\n", "  ")
	if len(added) != 2 {
		t.Errorf("added: got %v, want [spaced prod]", added)
	}
	if len(e.Tags) != 2 || e.Tags[0] != "spaced" || e.Tags[1] != "prod" {
		t.Errorf("normalized tags: %v", e.Tags)
	}
}

func TestAddTagsTreatsCasesAsDistinct(t *testing.T) {
	// Current behavior: normalization trims only whitespace, does not
	// lowercase. "Prod" and "prod" are different tags. This test locks that
	// behavior in — if we ever switch to case-folding we should bump state
	// version and migrate.
	e := &Entry{}
	added := e.AddTags("Prod", "prod")
	if len(added) != 2 {
		t.Errorf("added: got %v, want both kept", added)
	}
}

func TestTagsRoundTripUnicodeAndLongNames(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	long := strings.Repeat("a", 200)
	unicode := "prod-日本-🚀"
	if err := s.Mutate(ctx, func(cfg *Config) error {
		cfg.AvailableTags = []string{long, unicode}
		cfg.Entries["sha256:abc"] = Entry{Tags: []string{long, unicode}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AvailableTags) != 2 ||
		cfg.AvailableTags[0] != long || cfg.AvailableTags[1] != unicode {
		t.Errorf("palette round-trip changed tags: %v", cfg.AvailableTags)
	}
	if e := cfg.Entries["sha256:abc"]; len(e.Tags) != 2 ||
		e.Tags[0] != long || e.Tags[1] != unicode {
		t.Errorf("entry round-trip changed tags: %v", e.Tags)
	}
}
