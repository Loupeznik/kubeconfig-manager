package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

const prodFixture = `apiVersion: v1
kind: Config
current-context: prod-eu
clusters:
  - name: prod-eu-cluster
    cluster:
      server: https://prod-eu.example.test
      insecure-skip-tls-verify: true
  - name: prod-us-cluster
    cluster:
      server: https://prod-us.example.test
      insecure-skip-tls-verify: true
contexts:
  - name: prod-eu
    context:
      cluster: prod-eu-cluster
      user: prod-admin
      namespace: default
  - name: prod-us
    context:
      cluster: prod-us-cluster
      user: prod-admin
users:
  - name: prod-admin
    user:
      token: test-token
`

const stagingFixture = `apiVersion: v1
kind: Config
current-context: staging
clusters:
  - name: staging-cluster
    cluster:
      server: https://staging.example.test
contexts:
  - name: staging
    context:
      cluster: staging-cluster
      user: staging-user
users:
  - name: staging-user
    user:
      token: test-token
`

// isolateState routes all state reads/writes to a throwaway XDG_CONFIG_HOME
// and verifies the isolation actually took effect. The adrg/xdg package caches
// paths at init time, so t.Setenv alone is not enough — Reload() picks up the
// updated environment. The verification step guards against accidentally
// writing to the real user state dir.
func isolateState(t *testing.T, stateHome string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", stateHome)
	xdg.Reload()

	gotPath, err := state.DefaultPath()
	if err != nil {
		t.Fatalf("resolve default state path: %v", err)
	}
	if !strings.HasPrefix(gotPath, stateHome) {
		t.Fatalf("state isolation failed: path %q is not under %q", gotPath, stateHome)
	}
}

// runCmd executes the root command with args, capturing output. Isolates state
// to a throwaway XDG_CONFIG_HOME.
func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	stateHome := t.TempDir()
	isolateState(t, stateHome)

	root := NewRootCmd()
	root.SetArgs(args)
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	err = root.ExecuteContext(context.Background())
	return out.String(), errBuf.String(), err
}

// runCmdInState executes the root command against the same state file across
// invocations — use when a test needs to chain commands (e.g. palette add →
// tag add).
func runCmdInState(t *testing.T, stateHome string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	isolateState(t, stateHome)

	root := NewRootCmd()
	root.SetArgs(args)
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	err = root.ExecuteContext(context.Background())
	return out.String(), errBuf.String(), err
}

func seedKubeconfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeKubeconfig(t, dir, "prod.yaml", prodFixture)
	writeKubeconfig(t, dir, "staging.yaml", stagingFixture)
	return dir
}

func writeKubeconfig(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// -------- list ---------------------------------------------------------------

func TestListShowsFilesInDir(t *testing.T) {
	dir := seedKubeconfigDir(t)
	out, _, err := runCmd(t, "list", "--dir", dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"prod.yaml", "staging.yaml", "prod-eu", "staging", "CONTEXTS"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q\n%s", want, out)
		}
	}
}

func TestListEmptyDirReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	_, errBuf, err := runCmd(t, "list", "--dir", dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(errBuf, "no kubeconfig files found") {
		t.Errorf("expected 'no kubeconfig files found' in stderr, got: %s", errBuf)
	}
}

func TestListQuietAboutParseFailuresByDefault(t *testing.T) {
	dir := seedKubeconfigDir(t)
	writeKubeconfig(t, dir, "notes.txt", "this is not a kubeconfig\n")

	out, errBuf, err := runCmd(t, "list", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "notes.txt") || strings.Contains(errBuf, "notes.txt") {
		t.Errorf("quiet default should not mention unparseable files:\nout=%s\nerr=%s", out, errBuf)
	}
}

func TestListVerboseShowsSkippedFiles(t *testing.T) {
	dir := seedKubeconfigDir(t)
	writeKubeconfig(t, dir, "notes.txt", "nope\n")

	_, errBuf, err := runCmd(t, "list", "--dir", dir, "--verbose")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errBuf, "notes.txt") {
		t.Errorf("--verbose should surface skipped files, got stderr: %s", errBuf)
	}
}

// -------- show & contexts ----------------------------------------------------

func TestShowRendersContextsAndClusters(t *testing.T) {
	dir := seedKubeconfigDir(t)
	out, _, err := runCmd(t, "show", "prod", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"prod-eu", "prod-us", "prod-eu-cluster", "prod-admin",
		"https://prod-eu.example.test",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q\n%s", want, out)
		}
	}
}

func TestShowBareNameResolvesWithExtension(t *testing.T) {
	dir := seedKubeconfigDir(t)
	// "prod" should resolve to "prod.yaml"
	out, _, err := runCmd(t, "show", "prod", "--dir", dir)
	if err != nil {
		t.Fatalf("bare name resolution failed: %v", err)
	}
	if !strings.Contains(out, "prod.yaml") {
		t.Errorf("expected prod.yaml path in output: %s", out)
	}
}

// -------- use (shell export) -------------------------------------------------

func TestUseEmitsBashExport(t *testing.T) {
	dir := seedKubeconfigDir(t)
	out, _, err := runCmd(t, "use", "prod", "--dir", dir, "--shell=bash")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "export KUBECONFIG=") {
		t.Errorf("expected bash export, got: %s", out)
	}
	if !strings.Contains(out, "prod.yaml") {
		t.Errorf("expected prod.yaml in export line: %s", out)
	}
}

func TestUseEmitsPowerShellExport(t *testing.T) {
	dir := seedKubeconfigDir(t)
	out, _, err := runCmd(t, "use", "prod", "--dir", dir, "--shell=pwsh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "$env:KUBECONFIG =") {
		t.Errorf("expected pwsh export, got: %s", out)
	}
}

func TestUseUnknownShellErrors(t *testing.T) {
	dir := seedKubeconfigDir(t)
	_, _, err := runCmd(t, "use", "prod", "--dir", dir, "--shell=fish")
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

// -------- tag palette lifecycle ---------------------------------------------

func TestTagPaletteAddListRemove(t *testing.T) {
	stateHome := t.TempDir()

	// Add three tags to palette.
	out, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod", "staging", "critical")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "added to palette: prod, staging, critical") {
		t.Errorf("add output: %s", out)
	}

	// List should show all three.
	out, _, err = runCmdInState(t, stateHome, "tag", "palette", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, t2 := range []string{"prod", "staging", "critical"} {
		if !strings.Contains(out, t2) {
			t.Errorf("palette list missing %q: %s", t2, out)
		}
	}

	// Remove one.
	out, _, err = runCmdInState(t, stateHome, "tag", "palette", "remove", "staging")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "removed from palette") || !strings.Contains(out, "staging") {
		t.Errorf("remove output: %s", out)
	}

	out, _, err = runCmdInState(t, stateHome, "tag", "palette", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "staging") {
		t.Errorf("palette should no longer contain staging: %s", out)
	}
}

func TestTagAddRejectsUnknownTagByDefault(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	// Seed palette so the validation kicks in (empty palette is permissive).
	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod"); err != nil {
		t.Fatal(err)
	}
	_, _, err := runCmdInState(t, stateHome, "tag", "add", "prod", "unknown-tag", "--dir", dir)
	if err == nil {
		t.Fatal("expected error for unknown tag without --allow-new")
	}
	if !strings.Contains(err.Error(), "not in palette") {
		t.Errorf("error should mention palette: %v", err)
	}
}

func TestTagAddAllowNewExtendsPalette(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "tag", "add", "prod", "ephemeral", "--allow-new", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "added to palette: ephemeral") {
		t.Errorf("expected palette extension, got: %s", out)
	}

	// Verify ephemeral is now in the palette.
	out, _, err = runCmdInState(t, stateHome, "tag", "palette", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ephemeral") {
		t.Errorf("palette list missing ephemeral: %s", out)
	}
}

func TestTagAddPersistsFileLevelAssignment(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod", "eu"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "tag", "add", "prod", "prod", "eu", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	// Read state directly and check.
	storePath := filepath.Join(stateHome, "kubeconfig-manager", "config.yaml")
	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("state not written: %v", err)
	}
	if !strings.Contains(string(data), "- prod") || !strings.Contains(string(data), "- eu") {
		t.Errorf("state missing tags:\n%s", string(data))
	}
}

// -------- alerts -------------------------------------------------------------

func TestAlertEnableFileLevel(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "alert", "show", "prod", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "File-level") || !strings.Contains(out, "Enabled:               true") {
		t.Errorf("expected file-level alerts enabled: %s", out)
	}
}

func TestAlertEnablePerContext(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--context", "prod-eu", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "alert", "show", "prod", "--context", "prod-eu", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Context prod-eu") || !strings.Contains(out, "Enabled:               true") {
		t.Errorf("expected per-context alerts enabled: %s", out)
	}
}

func TestAlertShowWithoutEntryFallsBackToDefaults(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	out, _, err := runCmdInState(t, stateHome, "alert", "show", "prod", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	// Defaults: Enabled=false, blocked_verbs populated from DefaultBlockedVerbs
	if !strings.Contains(out, "Enabled:               false") {
		t.Errorf("default should be disabled: %s", out)
	}
	if !strings.Contains(out, "delete") {
		t.Errorf("default blocked verbs should include delete: %s", out)
	}
}

// -------- import/split/merge -------------------------------------------------

func TestImportMergesIntoDestination(t *testing.T) {
	dir := seedKubeconfigDir(t)
	dest := filepath.Join(dir, "dest.yaml")
	src := filepath.Join(dir, "staging.yaml")

	// Start with an empty dest.
	if _, _, err := runCmd(t, "import", src, "--into", dest); err != nil {
		t.Fatal(err)
	}
	f, err := kubeconfig.Load(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Config.Contexts["staging"]; !ok {
		t.Errorf("dest missing staging context: %v", f.Config.Contexts)
	}
}

func TestSplitExtractsSingleContext(t *testing.T) {
	dir := seedKubeconfigDir(t)
	out := filepath.Join(dir, "split.yaml")

	if _, _, err := runCmd(t, "split", "prod-eu", out, "--from", filepath.Join(dir, "prod.yaml")); err != nil {
		t.Fatal(err)
	}
	f, err := kubeconfig.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Config.Contexts) != 1 || f.Config.Contexts["prod-eu"] == nil {
		t.Errorf("expected only prod-eu context: %v", f.Config.Contexts)
	}
	if f.Config.CurrentContext != "prod-eu" {
		t.Errorf("current-context should be the extracted one: %s", f.Config.CurrentContext)
	}
}

func TestMergeRejectsCollisionsByDefault(t *testing.T) {
	dir := seedKubeconfigDir(t)
	p := filepath.Join(dir, "prod.yaml")
	out := filepath.Join(dir, "merged.yaml")

	_, _, err := runCmd(t, "merge", p, p, out)
	if err == nil {
		t.Fatal("expected collision error when merging a file with itself")
	}
	if !strings.Contains(err.Error(), "collision") {
		t.Errorf("error should mention collisions: %v", err)
	}
}

func TestMergeWithSkipPolicy(t *testing.T) {
	dir := seedKubeconfigDir(t)
	p := filepath.Join(dir, "prod.yaml")
	s := filepath.Join(dir, "staging.yaml")
	out := filepath.Join(dir, "merged.yaml")

	if _, _, err := runCmd(t, "merge", p, s, out, "--on-conflict=skip"); err != nil {
		t.Fatal(err)
	}
	f, err := kubeconfig.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Config.Contexts) != 3 {
		t.Errorf("expected 3 contexts (2 from prod + 1 from staging), got %d: %v",
			len(f.Config.Contexts), f.Config.Contexts)
	}
}

// -------- shell hook installer (atomic, idempotent) -------------------------

func TestInstallShellHookCreatesIdempotentBlock(t *testing.T) {
	rc := filepath.Join(t.TempDir(), "fake.zshrc")
	if _, _, err := runCmd(t, "install-shell-hook", "--shell=zsh", "--rc", rc); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), ">>> kubeconfig-manager shell hook >>>") {
		t.Errorf("fence markers missing: %s", first)
	}
	if !strings.Contains(string(first), "alias kubectl=") {
		t.Errorf("alias should be on by default: %s", first)
	}

	// Re-installing with --no-alias-kubectl should replace the block, not append.
	if _, _, err := runCmd(t, "install-shell-hook", "--shell=zsh", "--rc", rc, "--no-alias-kubectl"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	fenceCount := strings.Count(string(second), ">>> kubeconfig-manager shell hook >>>")
	if fenceCount != 1 {
		t.Errorf("expected exactly one fence block, got %d", fenceCount)
	}
	if strings.Contains(string(second), "alias kubectl=") {
		t.Errorf("alias should be removed with --no-alias-kubectl: %s", second)
	}
}

// -------- state isolation sanity --------------------------------------------

func TestStateIsIsolatedViaXDGConfigHome(t *testing.T) {
	stateHome := t.TempDir()

	// Add a tag to palette.
	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "alpha"); err != nil {
		t.Fatal(err)
	}

	// Verify state file exists exactly where XDG says it should.
	store := state.NewFileStore(filepath.Join(stateHome, "kubeconfig-manager", "config.yaml"))
	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AvailableTags) != 1 || cfg.AvailableTags[0] != "alpha" {
		t.Errorf("state not isolated correctly: %v", cfg.AvailableTags)
	}
}
