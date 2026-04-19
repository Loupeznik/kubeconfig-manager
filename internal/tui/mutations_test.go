package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

const tuiTestKubeconfig = `apiVersion: v1
kind: Config
current-context: ctx-a
clusters:
  - name: c1
    cluster:
      server: https://c1.example.test
contexts:
  - name: ctx-a
    context:
      cluster: c1
      user: admin
  - name: ctx-b
    context:
      cluster: c1
      user: admin
users:
  - name: admin
    user:
      token: x
`

// TestSetTagsPerContextCreatesExclusionWhenInheritedTagDeselected simulates
// the TUI flow: file-level tags [prod, eu, critical] are assigned; the user
// opens the picker on ctx-b with the effective list pre-selected, deselects
// "eu", and hits save. The resulting state must have an exclusion entry so
// ResolveTags("ctx-b") no longer returns "eu" — while ctx-a still inherits it.
func TestSetTagsPerContextCreatesExclusionWhenInheritedTagDeselected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prod.yaml")
	if err := os.WriteFile(path, []byte(tuiTestKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	store := state.NewFileStore(filepath.Join(t.TempDir(), "config.yaml"))
	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.AvailableTags = []string{"prod", "eu", "critical"}
		cfg.Entries[id.StableHash] = state.Entry{
			PathHint: "prod.yaml",
			Tags:     []string{"prod", "eu", "critical"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate picker save on ctx-b: user deselected "eu".
	if err := setTags(store, id, "prod.yaml", "ctx-b", []string{"prod", "critical"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entry := cfg.Entries[id.StableHash]
	if excl := entry.ContextTagExclusions["ctx-b"]; len(excl) != 1 || excl[0] != "eu" {
		t.Errorf("ctx-b exclusions: got %v, want [eu]", excl)
	}
	if len(entry.ContextTags["ctx-b"]) != 0 {
		t.Errorf("ctx-b context_tags: got %v, want empty", entry.ContextTags["ctx-b"])
	}
	// File-level unchanged.
	if len(entry.Tags) != 3 {
		t.Errorf("file-level tags should stay at 3: %v", entry.Tags)
	}
	// Effective for ctx-b no longer contains eu.
	eff := entry.ResolveTags("ctx-b")
	for _, tag := range eff {
		if tag == "eu" {
			t.Errorf("ctx-b effective tags should not include eu: %v", eff)
		}
	}
	// ctx-a still inherits all three.
	if got := entry.ResolveTags("ctx-a"); len(got) != 3 {
		t.Errorf("ctx-a effective tags: got %v, want 3", got)
	}
}

// TestSetTagsPerContextReaddingInheritedTagClearsExclusion covers the inverse:
// after excluding "eu" from ctx-b, the user reopens the picker and re-selects
// "eu". The exclusion must drop so the tag is inherited again.
func TestSetTagsPerContextReaddingInheritedTagClearsExclusion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prod.yaml")
	if err := os.WriteFile(path, []byte(tuiTestKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	store := state.NewFileStore(filepath.Join(t.TempDir(), "config.yaml"))
	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.AvailableTags = []string{"prod", "eu", "critical"}
		cfg.Entries[id.StableHash] = state.Entry{
			PathHint:             "prod.yaml",
			Tags:                 []string{"prod", "eu", "critical"},
			ContextTagExclusions: map[string][]string{"ctx-b": {"eu"}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// User re-selects eu, saves full effective set back.
	if err := setTags(store, id, "prod.yaml", "ctx-b", []string{"prod", "eu", "critical"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entry := cfg.Entries[id.StableHash]
	if excl, has := entry.ContextTagExclusions["ctx-b"]; has && len(excl) > 0 {
		t.Errorf("ctx-b exclusions should have cleared, got %v", excl)
	}
	if got := entry.ResolveTags("ctx-b"); len(got) != 3 {
		t.Errorf("ctx-b effective tags after re-add: got %v, want 3", got)
	}
}

// TestSetTagsPerContextAddingNewTagBecomesContextTag covers the basic "add a
// new tag that isn't file-level" path — it should land in ContextTags, not
// get written back to the file-level Tags list.
func TestSetTagsPerContextAddingNewTagBecomesContextTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prod.yaml")
	if err := os.WriteFile(path, []byte(tuiTestKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	store := state.NewFileStore(filepath.Join(t.TempDir(), "config.yaml"))
	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.AvailableTags = []string{"prod", "eu", "critical", "shiny"}
		cfg.Entries[id.StableHash] = state.Entry{
			PathHint: "prod.yaml",
			Tags:     []string{"prod", "eu", "critical"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := setTags(store, id, "prod.yaml", "ctx-b",
		[]string{"prod", "eu", "critical", "shiny"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entry := cfg.Entries[id.StableHash]
	if got := entry.ContextTags["ctx-b"]; len(got) != 1 || got[0] != "shiny" {
		t.Errorf("ctx-b context_tags: got %v, want [shiny]", got)
	}
	if len(entry.ContextTagExclusions["ctx-b"]) != 0 {
		t.Errorf("no exclusions expected: %v", entry.ContextTagExclusions["ctx-b"])
	}
	if len(entry.Tags) != 3 {
		t.Errorf("file-level tags should not mutate: %v", entry.Tags)
	}
}
