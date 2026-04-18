package guard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

const sampleKubeconfig = `apiVersion: v1
kind: Config
current-context: prod
clusters:
  - name: prod-cluster
    cluster:
      server: https://prod.example.test
contexts:
  - name: prod
    context:
      cluster: prod-cluster
      user: prod-user
users:
  - name: prod-user
    user:
      token: x
`

func writeKubeconfig(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(sampleKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestStore(t *testing.T) *state.FileStore {
	t.Helper()
	return state.NewFileStore(filepath.Join(t.TempDir(), "state.yaml"))
}

func seedEntry(t *testing.T, store *state.FileStore, path string, entry state.Entry) {
	t.Helper()
	hash, err := kubeconfig.StableHashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.Entries[hash] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateNoKubeconfigReturnsEmptyDecision(t *testing.T) {
	store := newTestStore(t)
	d, err := Evaluate(context.Background(), store, "", []string{"delete", "pod"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Errorf("alert fired with no kubeconfig: %+v", d)
	}
}

func TestEvaluateAlertsDisabledNoTrigger(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: false},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"delete", "pod"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("alert fired with alerts disabled")
	}
}

func TestEvaluateVerbNotInBlockedListNoTrigger(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{
			Enabled:      true,
			BlockedVerbs: []string{"delete"},
		},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"get", "pods"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("alert fired on safe verb")
	}
}

func TestEvaluateTriggersOnBlockedVerb(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{
			Enabled:             true,
			RequireConfirmation: true,
			BlockedVerbs:        []string{"delete", "drain"},
		},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"delete", "pod", "mypod"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected alert to fire")
	}
	if d.Verb != "delete" {
		t.Errorf("verb: got %q, want delete", d.Verb)
	}
	if !d.RequireConfirm() {
		t.Error("expected RequireConfirm=true")
	}
	if len(d.Triggers) != 1 {
		t.Errorf("triggers: got %d, want 1", len(d.Triggers))
	}
	if d.Triggers[0].ClusterName != "prod-cluster" {
		t.Errorf("cluster: got %q", d.Triggers[0].ClusterName)
	}
	if d.Triggers[0].ContextName != "prod" {
		t.Errorf("context: got %q", d.Triggers[0].ContextName)
	}
}

func TestEvaluateDefaultBlockedVerbsWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: true},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"drain", "node-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Error("expected default blocked verbs to trigger drain")
	}
}

func TestEvaluateMultiPathKubeconfig(t *testing.T) {
	dir := t.TempDir()
	path1 := writeKubeconfig(t, dir, "prod.yaml")
	// path2 has a different cluster server URL so its stable fingerprint
	// differs from path1's — otherwise stable-hash keying treats two files
	// with the same logical topology as the same kubeconfig.
	path2 := filepath.Join(dir, "other.yaml")
	otherKubeconfig := strings.Replace(sampleKubeconfig,
		"server: https://prod.example.test",
		"server: https://other.example.test", 1)
	if err := os.WriteFile(path2, []byte(otherKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newTestStore(t)
	seedEntry(t, store, path1, state.Entry{
		Alerts: state.Alerts{Enabled: true, BlockedVerbs: []string{"delete"}},
	})

	multi := path1 + string(os.PathListSeparator) + path2
	d, err := Evaluate(context.Background(), store, multi, []string{"delete", "pod"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Error("expected alert from first path")
	}
	if len(d.Triggers) != 1 {
		t.Errorf("triggers: got %d, want 1 (path2 has distinct topology; no entry seeded)", len(d.Triggers))
	}
	if d.Triggers[0].Path != path1 {
		t.Errorf("trigger path: got %q, want %q", d.Triggers[0].Path, path1)
	}
}

func TestEvaluateNoVerbNoAlert(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: true, BlockedVerbs: []string{"delete"}},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("alert fired with no verb")
	}
}

func TestContextAlertOverrideWins(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	// File-level: disabled. Context-level (for the file's current-context "prod"): enabled for delete.
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: false},
		ContextAlerts: map[string]state.Alerts{
			"prod": {
				Enabled:             true,
				RequireConfirmation: true,
				BlockedVerbs:        []string{"delete"},
			},
		},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"delete", "pod", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected context-level override to trigger")
	}
	if d.Triggers[0].ContextName != "prod" {
		t.Errorf("context: got %q", d.Triggers[0].ContextName)
	}
}

func TestContextAlertExplicitDisableOverridesFileLevel(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	// File-level: enabled for delete. Context-level: explicitly disabled.
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: true, BlockedVerbs: []string{"delete"}},
		ContextAlerts: map[string]state.Alerts{
			"prod": {Enabled: false},
		},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"delete", "pod", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Fatal("context-level disable should suppress alert")
	}
}

func TestContextAlertFallsBackToFileLevelWhenNoOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	// File-level only; no context override.
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: true, BlockedVerbs: []string{"delete"}},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"delete", "pod", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected file-level policy to apply")
	}
}

func TestContextAlertHonorsArgContextFlag(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	// File-level: disabled. Context-level for "other" (NOT the current-context): enabled.
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: false},
		ContextAlerts: map[string]state.Alerts{
			"other": {Enabled: true, BlockedVerbs: []string{"delete"}},
		},
	})

	// When user passes --context=other, that context's policy should apply.
	d, err := Evaluate(context.Background(), store, path, []string{"--context=other", "delete", "pod"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected --context override to pick context-level policy")
	}
	if d.Triggers[0].ContextName != "other" {
		t.Errorf("context: got %q, want other", d.Triggers[0].ContextName)
	}
}

func TestContextAlertArgOverrideShadowsCurrentContextPolicy(t *testing.T) {
	// Current-context is "prod"; a --context flag points at "prod-us" which
	// has its own per-context override. The guard should honor prod-us's
	// policy, not prod's. Locks behavior: ExtractContext > current-context.
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	// Extend the kubeconfig with a second context so ResolveClusterName
	// doesn't drop the alert for an unknown context.
	extra := `- name: prod-us
    context:
      cluster: prod-cluster
      user: prod-user
`
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	augmented := strings.Replace(string(data),
		"contexts:\n  - name: prod",
		"contexts:\n  "+extra+"  - name: prod", 1)
	if err := os.WriteFile(path, []byte(augmented), 0o600); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	// File-level enabled for "get" (won't fire on delete). Per-context for
	// prod-us enables "delete". If the guard looked only at current-context,
	// "delete" wouldn't fire.
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{Enabled: true, BlockedVerbs: []string{"get"}},
		ContextAlerts: map[string]state.Alerts{
			"prod-us": {Enabled: true, BlockedVerbs: []string{"delete"}},
		},
	})

	d, err := Evaluate(context.Background(), store, path,
		[]string{"--context", "prod-us", "delete", "pod", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected prod-us context policy to fire via --context override")
	}
	if d.Triggers[0].ContextName != "prod-us" {
		t.Errorf("context: got %q, want prod-us", d.Triggers[0].ContextName)
	}
}

func TestExtractContext(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"delete", "pod"}, ""},
		{[]string{"--context", "prod", "delete"}, "prod"},
		{[]string{"--context=prod", "delete"}, "prod"},
		{[]string{"-n", "ns", "--context", "prod", "delete"}, "prod"},
		{[]string{"--context"}, ""},
	}
	for _, tt := range cases {
		got := ExtractContext(tt.args)
		if got != tt.want {
			t.Errorf("ExtractContext(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestRequireClusterName(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)
	seedEntry(t, store, path, state.Entry{
		Alerts: state.Alerts{
			Enabled:            true,
			ConfirmClusterName: true,
			BlockedVerbs:       []string{"delete"},
		},
	})

	d, err := Evaluate(context.Background(), store, path, []string{"delete", "pod"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.RequireClusterName() {
		t.Error("expected RequireClusterName=true")
	}
	expected := d.ExpectedClusterNames()
	if len(expected) != 1 || expected[0] != "prod-cluster" {
		t.Errorf("expected cluster names: got %v", expected)
	}
}
