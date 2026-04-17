package guard

import (
	"context"
	"os"
	"path/filepath"
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
	hash, err := kubeconfig.HashFile(path)
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
	path2 := filepath.Join(dir, "other.yaml")
	otherKubeconfig := sampleKubeconfig + "# distinct content for different hash\n"
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
		t.Errorf("triggers: got %d, want 1 (path2 has distinct content; no entry seeded)", len(d.Triggers))
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
