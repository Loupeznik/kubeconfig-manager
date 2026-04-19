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

func TestExtractHelmValuesPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"no flags", []string{"upgrade", "myrelease"}, nil},
		{"short flag", []string{"upgrade", "-f", "a.yaml"}, []string{"a.yaml"}},
		{"long flag", []string{"upgrade", "--values", "a.yaml"}, []string{"a.yaml"}},
		{"equals form", []string{"upgrade", "--values=a.yaml"}, []string{"a.yaml"}},
		{"short equals form", []string{"upgrade", "-f=a.yaml"}, []string{"a.yaml"}},
		{"comma list", []string{"upgrade", "-f", "a.yaml,b.yaml,c.yaml"}, []string{"a.yaml", "b.yaml", "c.yaml"}},
		{"multiple flags", []string{"upgrade", "-f", "a.yaml", "-f", "b.yaml"}, []string{"a.yaml", "b.yaml"}},
		{"mixed", []string{"upgrade", "--values=a.yaml", "-f", "b.yaml"}, []string{"a.yaml", "b.yaml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHelmValuesPaths(tt.args)
			if !equalSlices(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDeriveNameFromPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		pattern string
		want    string
		ok      bool
	}{
		{"default pattern match", "envs/clusters/prod-eu/values.yaml", "clusters/{name}/", "prod-eu", true},
		{"default pattern deeper", "a/b/clusters/staging/c/values.yaml", "clusters/{name}/", "staging", true},
		{"pattern at end", "clusters/dev.yaml", "clusters/{name}.yaml", "dev", true},
		{"no match", "values/dev/values.yaml", "clusters/{name}/", "", false},
		{"empty pattern", "clusters/foo/v.yaml", "", "", false},
		{"pattern without placeholder", "clusters/foo/v.yaml", "clusters/abc/", "", false},
		{"windows path normalized", `C:\repo\clusters\prod\values.yaml`, "clusters/{name}/", "prod", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := deriveNameFromPath(tt.path, tt.pattern)
			if ok != tt.ok {
				t.Fatalf("ok: got %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("name: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeriveNameFromPatterns(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		patterns []string
		want     string
		ok       bool
	}{
		{"first pattern wins", "envs/prod-eu/values.yaml",
			[]string{"envs/{name}/", "clusters/{name}/"}, "prod-eu", true},
		{"second pattern matches", "repo/clusters/prod-eu/values.yaml",
			[]string{"envs/{name}/", "clusters/{name}/"}, "prod-eu", true},
		{"none match", "weird/layout/values.yaml",
			[]string{"envs/{name}/", "clusters/{name}/"}, "", false},
		{"empty list", "clusters/prod/values.yaml", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := deriveNameFromPatterns(tt.path, tt.patterns)
			if ok != tt.ok {
				t.Fatalf("ok: got %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("name: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompareHelmNamesHardMismatch(t *testing.T) {
	sev, reason := compareHelmNames("k8s-test-01", "k8s-prod-01", "k8s-prod-01", state.DefaultEnvTokens())
	if sev != HelmMatchHard {
		t.Errorf("severity: got %v, want HelmMatchHard", sev)
	}
	if reason == "" {
		t.Error("expected a reason")
	}
}

func TestCompareHelmNamesOKOnSharedEnvToken(t *testing.T) {
	sev, _ := compareHelmNames("k8s-prod-eu", "prod-eu-cluster", "prod-eu-cluster", state.DefaultEnvTokens())
	if sev != HelmMatchOK {
		t.Errorf("severity: got %v, want HelmMatchOK", sev)
	}
}

func TestCompareHelmNamesOKOnSharedNonEnvToken(t *testing.T) {
	// No env tokens on either side, but both contain "mycluster" — considered OK.
	sev, _ := compareHelmNames("mycluster-primary", "mycluster-main", "mycluster-main", state.DefaultEnvTokens())
	if sev != HelmMatchOK {
		t.Errorf("severity: got %v, want HelmMatchOK (overlap)", sev)
	}
}

func TestCompareHelmNamesSoftOnNoOverlap(t *testing.T) {
	sev, reason := compareHelmNames("ab-cd", "xy-zw", "other", state.DefaultEnvTokens())
	if sev != HelmMatchSoft {
		t.Errorf("severity: got %v, want HelmMatchSoft", sev)
	}
	if reason == "" {
		t.Error("expected a reason")
	}
}

func TestCompareHelmNamesOneSidedEnvStillOK(t *testing.T) {
	// Derived has env token, context has the same env — matches via env.
	sev, _ := compareHelmNames("dev-ui", "dev-api", "dev-api", state.DefaultEnvTokens())
	if sev != HelmMatchOK {
		t.Errorf("got %v, want HelmMatchOK", sev)
	}
}

// ---- Evaluate integration (uses real state store and real kubeconfig files) -

const helmKubeconfig = `apiVersion: v1
kind: Config
current-context: prod-eu
clusters:
  - name: prod-eu-cluster
    cluster:
      server: https://prod-eu.example.test
contexts:
  - name: prod-eu
    context:
      cluster: prod-eu-cluster
      user: admin
users:
  - name: admin
    user:
      token: x
`

func writeHelmKubeconfig(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(helmKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEvaluateHelmEnabledByDefault(t *testing.T) {
	// The guard is a safety feature and users expect it to be on out of the
	// box. An unconfigured state + the default pattern must still catch an
	// obvious prod/test mismatch.
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "clusters/test/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Error("helm-guard default should be on; alert did not fire on prod vs test")
	}
}

func TestEvaluateHelmExplicitlyDisabledSuppresses(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.HelmGuard = state.HelmGuard{Enabled: state.BoolPtr(false)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "clusters/test/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("explicit Enabled=false should suppress the default-on guard")
	}
}

func TestEvaluateHelmGlobalEnableCatchesMismatch(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	// Enable globally.
	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.HelmGuard = state.HelmGuard{Enabled: state.BoolPtr(true)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "repo/clusters/test-eu/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected alert on prod-vs-test mismatch")
	}
	if d.Triggers[0].Severity != HelmMatchHard {
		t.Errorf("severity: got %v, want HelmMatchHard", d.Triggers[0].Severity)
	}
}

func TestEvaluateHelmPerEntryOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	// Global enabled, per-entry disabled.
	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.HelmGuard = state.HelmGuard{Enabled: state.BoolPtr(true)}
		cfg.Entries[id.StableHash] = state.Entry{
			PathHint:  "prod.yaml",
			HelmGuard: &state.HelmGuard{Enabled: state.BoolPtr(false)},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "repo/clusters/test-eu/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("per-entry disable should suppress alert despite global enable")
	}
}

func TestEvaluateHelmCustomPatternPerEntry(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.Entries[id.StableHash] = state.Entry{
			PathHint: "prod.yaml",
			HelmGuard: &state.HelmGuard{
				Enabled:  state.BoolPtr(true),
				Patterns: []string{"envs/{name}/"},
			},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Path matches the custom pattern.
	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "deploy/envs/test-eu/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Error("expected alert with custom pattern")
	}

	// Path doesn't match the pattern — no derivation, no alert.
	d, err = EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "deploy/clusters/test-eu/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("path didn't match pattern; no alert should fire")
	}
}

func TestEvaluateHelmNoMismatchWhenPathMatchesContext(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.HelmGuard = state.HelmGuard{Enabled: state.BoolPtr(true)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "repo/clusters/prod-eu/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Errorf("no alert expected; triggers: %+v", d.Triggers)
	}
}

func TestEvaluateHelmMultiplePatternsPerEntry(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.Entries[id.StableHash] = state.Entry{
			PathHint: "prod.yaml",
			HelmGuard: &state.HelmGuard{
				Enabled:  state.BoolPtr(true),
				Patterns: []string{"envs/{name}/", "clusters/{name}/"},
			},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// First pattern matches this layout — hard mismatch fires.
	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "deploy/envs/test-eu/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Error("expected alert via first pattern")
	}

	// Second pattern matches this layout — also fires.
	d, err = EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "deploy/clusters/test-eu/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Error("expected alert via second pattern")
	}

	// Neither pattern matches and fallback is off — silent.
	d, err = EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "weird/layout/test-eu.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("no pattern matched and fallback disabled; alert should not fire")
	}
}

func TestEvaluateHelmGlobalFallbackCatchesIrregularLayout(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.HelmGuard = state.HelmGuard{
			Enabled:        state.BoolPtr(true),
			GlobalFallback: true,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Path doesn't match the default "clusters/{name}/" pattern, but the
	// fallback tokenizes the whole path and sees "test" vs the "prod" context.
	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "helm/my-app.test.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected alert from fallback tokenization")
	}
	if d.Triggers[0].Severity != HelmMatchHard {
		t.Errorf("severity: got %v, want HelmMatchHard", d.Triggers[0].Severity)
	}
}

func TestEvaluateHelmGlobalFallbackDisabledStillSilent(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.HelmGuard = state.HelmGuard{Enabled: state.BoolPtr(true)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Same irregular layout as above; without fallback, silence.
	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade", "-f", "helm/my-app.test.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Alert() {
		t.Error("fallback is off; no alert expected for unmatched path")
	}
}

func TestEvaluateHelmMultipleValuesFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeHelmKubeconfig(t, dir, "prod.yaml")
	store := newTestStore(t)

	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.HelmGuard = state.HelmGuard{Enabled: state.BoolPtr(true)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	d, err := EvaluateHelm(context.Background(), store, path,
		[]string{"upgrade",
			"-f", "clusters/prod-eu/values.yaml", // matches — no trigger
			"-f", "clusters/test-eu/overrides.yaml", // hard mismatch
		})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected alert from second values file")
	}
	if len(d.Triggers) != 1 {
		t.Errorf("triggers: got %d, want 1", len(d.Triggers))
	}
	if !strings.Contains(d.Triggers[0].ValuesPath, "test-eu") {
		t.Errorf("unexpected trigger path: %s", d.Triggers[0].ValuesPath)
	}
}
