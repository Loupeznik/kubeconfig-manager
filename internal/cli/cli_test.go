package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"

	"github.com/loupeznik/kubeconfig-manager/internal/guard"
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
func isolateState(t testing.TB, stateHome string) {
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

func seedKubeconfigDir(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	writeKubeconfig(t, dir, "prod.yaml", prodFixture)
	writeKubeconfig(t, dir, "staging.yaml", stagingFixture)
	return dir
}

func writeKubeconfig(t testing.TB, dir, name, content string) {
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
	_, _, err := runCmd(t, "use", "prod", "--dir", dir, "--shell=tcsh")
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

// -------- init --------------------------------------------------------------

func TestInitYesSeedsPaletteAndInstallsHook(t *testing.T) {
	stateHome := t.TempDir()
	rc := filepath.Join(t.TempDir(), "rc.sh")

	out, _, err := runCmdInState(t, stateHome, "init", "--yes", "--rc", rc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"added to palette",
		"created " + rc,
		"Next steps",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("init output missing %q:\n%s", want, out)
		}
	}
	// Palette actually written — follow-up command sees it.
	out, _, err = runCmdInState(t, stateHome, "tag", "palette", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "prod") {
		t.Errorf("palette missing seeded tags: %s", out)
	}
	// rc file contains the hook fence.
	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "kubeconfig-manager shell hook") {
		t.Errorf("rc missing hook block: %s", string(data))
	}
}

func TestInitSkipFlagsDisableSteps(t *testing.T) {
	stateHome := t.TempDir()
	rc := filepath.Join(t.TempDir(), "rc.sh")

	out, _, err := runCmdInState(t, stateHome, "init", "--yes", "--skip-palette", "--skip-shell-hook", "--rc", rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[skip] palette") || !strings.Contains(out, "[skip] shell hook") {
		t.Errorf("expected both skips reported: %s", out)
	}
	if _, err := os.Stat(rc); err == nil {
		t.Errorf("rc file should not have been created when --skip-shell-hook is set")
	}
}

// -------- dry-run -----------------------------------------------------------

func TestDryRunSkipsFileAndStateWrites(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	out, _, err := runCmdInState(t, stateHome, "context", "rename", "prod", "prod-eu", "prod-europe", "--dir", dir, "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("expected dry-run prefix: %s", out)
	}
	// File stayed on disk unchanged — prod-eu still present, prod-europe not.
	f, err := kubeconfig.Load(filepath.Join(dir, "prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, has := f.Config.Contexts["prod-eu"]; !has {
		t.Error("dry-run should leave original context in place")
	}
	if _, has := f.Config.Contexts["prod-europe"]; has {
		t.Error("dry-run should not have created the new context")
	}

	// Same for file rename.
	out, _, err = runCmdInState(t, stateHome, "rename", "prod", "production.yaml", "--dir", dir, "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[dry-run]") {
		t.Errorf("expected dry-run prefix: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "prod.yaml")); err != nil {
		t.Error("dry-run should leave original filename in place")
	}
	if _, err := os.Stat(filepath.Join(dir, "production.yaml")); err == nil {
		t.Error("dry-run should not have created new filename")
	}
}

// -------- doctor ------------------------------------------------------------

func TestDoctorReportsCheckLines(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "prod.yaml"))

	out, _, err := runCmdInState(t, stateHome, "doctor")
	// No assertion on err — missing helm/kubectl on a contributor's machine
	// would make this fail for unrelated reasons. We only assert format.
	_ = err
	for _, section := range []string{
		"kubectl on PATH",
		"shell hook",
		"state file",
		"active kubeconfig",
		"tag palette",
		"stale state entries",
	} {
		if !strings.Contains(out, section) {
			t.Errorf("doctor output missing section %q:\n%s", section, out)
		}
	}
}

// -------- starship ----------------------------------------------------------

func TestStarshipSilentWhenNoEntry(t *testing.T) {
	dir := seedKubeconfigDir(t)
	t.Setenv("KUBECONFIG", filepath.Join(dir, "prod.yaml"))

	out, _, err := runCmd(t, "starship")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output when no state entry, got: %q", out)
	}
}

func TestStarshipShowsTagsOnly(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()
	prod := filepath.Join(dir, "prod.yaml")
	t.Setenv("KUBECONFIG", prod)

	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod", "eu"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "tag", "add", "prod", "prod", "eu", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCmdInState(t, stateHome, "starship")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(out)
	if strings.Contains(line, "⚠") {
		t.Errorf("no alerts enabled; output should not include warning: %q", line)
	}
	if !strings.Contains(line, "prod") || !strings.Contains(line, "eu") {
		t.Errorf("output should include tags: %q", line)
	}
}

func TestStarshipShowsAlertsOnly(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "prod.yaml"))

	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCmdInState(t, stateHome, "starship")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(out)
	if line != "⚠" {
		t.Errorf("expected just the warning symbol, got: %q", line)
	}
}

func TestStarshipShowsBoth(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "prod.yaml"))

	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "tag", "add", "prod", "prod", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCmdInState(t, stateHome, "starship")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(out)
	if !strings.HasPrefix(line, "⚠") {
		t.Errorf("expected output to start with warning: %q", line)
	}
	if !strings.Contains(line, "prod") {
		t.Errorf("expected tag prod in output: %q", line)
	}
}

func TestStarshipHonorsContextFlag(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "prod.yaml"))

	// Enable alerts only for prod-us context.
	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--context", "prod-us", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	// Default current-context is prod-eu → no alert.
	out, _, err := runCmdInState(t, stateHome, "starship")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.TrimSpace(out), "⚠") {
		t.Errorf("prod-eu should not have alerts enabled: %q", out)
	}

	// Explicit --context=prod-us → alert fires.
	out, _, err = runCmdInState(t, stateHome, "starship", "--context", "prod-us")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.TrimSpace(out), "⚠") {
		t.Errorf("prod-us should have alerts enabled: %q", out)
	}
}

func TestStarshipSilentWhenKubeconfigMissing(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/kubeconfig.yaml")

	out, _, err := runCmd(t, "starship")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("should be silent for missing file, got: %q", out)
	}
}

// -------- helm-guard ---------------------------------------------------------

func TestHelmGuardDefaultShow(t *testing.T) {
	stateHome := t.TempDir()

	out, _, err := runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Global") || !strings.Contains(out, "Enabled:          true (default)") {
		t.Errorf("default global helm-guard should be enabled-by-default, got: %s", out)
	}
	if !strings.Contains(out, "clusters/{name}/") {
		t.Errorf("default pattern missing: %s", out)
	}
}

func TestHelmGuardDefaultOnFiresWithoutExplicitEnable(t *testing.T) {
	// The whole point of the default-on change: a fresh install should catch
	// a prod/test mismatch without anyone running `kcm helm-guard enable`.
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()
	isolateState(t, stateHome)

	prod := filepath.Join(dir, "prod.yaml")

	store, err := state.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	// Do NOT call `helm-guard enable` — we specifically want the default-on
	// behavior to catch a mismatch.
	d, err := guard.EvaluateHelm(context.Background(), store, prod,
		[]string{"upgrade", "-f", "repo/clusters/k8s-test-01/values.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Alert() {
		t.Fatal("expected default-on helm-guard to fire on prod/test mismatch")
	}
}

func TestHelmGuardGlobalEnableDisable(t *testing.T) {
	stateHome := t.TempDir()

	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "enable"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	// After explicit enable, the "(default)" suffix drops and the boolean
	// renders as plain "true".
	if !strings.Contains(out, "Enabled:          true\n") {
		t.Errorf("expected explicit enabled, got: %s", out)
	}

	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "disable"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Enabled:          false\n") {
		t.Errorf("expected explicit disabled after disable, got: %s", out)
	}
}

func TestHelmGuardPerFileOverride(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	// Global on, per-file off.
	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "enable"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "disable", "--file", "prod", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "helm-guard", "show", "--file", "prod", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Per-entry override") {
		t.Errorf("expected per-entry override section: %s", out)
	}
	if !strings.Contains(out, "Effective (resolved)") {
		t.Errorf("expected resolved section: %s", out)
	}
}

func TestHelmGuardSetPatternValidates(t *testing.T) {
	stateHome := t.TempDir()

	_, _, err := runCmdInState(t, stateHome, "helm-guard", "set-pattern", "no-placeholder")
	if err == nil {
		t.Fatal("expected error for pattern without {name}")
	}
	if !strings.Contains(err.Error(), "{name}") {
		t.Errorf("error should mention {name}: %v", err)
	}

	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "set-pattern", "environments/{name}/"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "environments/{name}/") {
		t.Errorf("pattern not persisted: %s", out)
	}
}

func TestHelmGuardMultiplePatternsAndFallback(t *testing.T) {
	stateHome := t.TempDir()

	// Replace the default list with two patterns.
	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "set-patterns",
		"envs/{name}/", "clusters/{name}/"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "envs/{name}/") || !strings.Contains(out, "clusters/{name}/") {
		t.Errorf("both patterns should appear in show output: %s", out)
	}

	// Add one more.
	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "add-pattern", "deploy/{name}.yaml"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "deploy/{name}.yaml") {
		t.Errorf("added pattern missing: %s", out)
	}

	// Remove one.
	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "remove-pattern", "envs/{name}/"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "envs/{name}/") {
		t.Errorf("removed pattern still present: %s", out)
	}

	// Toggle fallback.
	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "fallback", "on"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Global fallback:  true") {
		t.Errorf("fallback should be on: %s", out)
	}

	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "fallback", "off"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCmdInState(t, stateHome, "helm-guard", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Global fallback:  false") {
		t.Errorf("fallback should be off: %s", out)
	}

	// Invalid pattern (no placeholder) rejected.
	if _, _, err := runCmdInState(t, stateHome, "helm-guard", "add-pattern", "no-placeholder"); err == nil {
		t.Error("expected error for pattern without {name}")
	}
}

// -------- context rename / delete -------------------------------------------

func TestContextRenameMovesStateKeys(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	// Seed palette + per-context tag + per-context alert on prod-eu.
	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod", "eu"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "tag", "add", "prod", "prod", "--context", "prod-eu", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--context", "prod-eu", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	// Rename prod-eu -> prod-europe.
	if _, _, err := runCmdInState(t, stateHome, "context", "rename", "prod", "prod-eu", "prod-europe", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	// Check the kubeconfig file renamed the context.
	f, err := kubeconfig.Load(filepath.Join(dir, "prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Config.Contexts["prod-europe"]; !ok {
		t.Errorf("kubeconfig missing renamed context: %v", f.Config.Contexts)
	}
	if _, ok := f.Config.Contexts["prod-eu"]; ok {
		t.Errorf("kubeconfig still has old context name")
	}
	if f.Config.CurrentContext != "prod-europe" {
		t.Errorf("current-context not updated: %q", f.Config.CurrentContext)
	}

	// Check state moved per-context alert + tags to the new key.
	out, _, err := runCmdInState(t, stateHome, "alert", "show", "prod", "--context", "prod-europe", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Enabled:               true") {
		t.Errorf("alerts should have moved to new context name: %s", out)
	}
	out, _, err = runCmdInState(t, stateHome, "tag", "list", "prod", "--context", "prod-europe", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "prod") {
		t.Errorf("tags should have moved to new context name: %s", out)
	}
}

func TestContextDeleteScrubsStateAndPrunesOrphans(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	if _, _, err := runCmdInState(t, stateHome, "tag", "palette", "add", "prod"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmdInState(t, stateHome, "tag", "add", "prod", "prod", "--context", "prod-us", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	// Delete prod-us. Default mode should also prune prod-us-cluster (now
	// unreferenced) since prod-eu stays with prod-admin which prod-us shared.
	if _, _, err := runCmdInState(t, stateHome, "context", "delete", "prod", "prod-us", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	f, err := kubeconfig.Load(filepath.Join(dir, "prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Config.Contexts["prod-us"]; ok {
		t.Error("context should be deleted")
	}
	if _, ok := f.Config.Clusters["prod-us-cluster"]; ok {
		t.Error("unreferenced prod-us-cluster should be pruned")
	}
	if _, ok := f.Config.AuthInfos["prod-admin"]; !ok {
		t.Error("prod-admin is still referenced by prod-eu; should stay")
	}

	// Per-context tags for prod-us scrubbed.
	out, _, err := runCmdInState(t, stateHome, "tag", "list", "prod", "--context", "prod-us", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no tags") {
		t.Errorf("context tags not scrubbed: %s", out)
	}
}

func TestContextDeleteKeepOrphansPreservesClusterAndUser(t *testing.T) {
	dir := seedKubeconfigDir(t)

	// staging.yaml has 1 context, 1 cluster, 1 user. --keep-orphans should
	// leave all three in place.
	if _, _, err := runCmd(t, "context", "delete", "staging", "staging", "--dir", dir, "--keep-orphans"); err != nil {
		t.Fatal(err)
	}
	f, err := kubeconfig.Load(filepath.Join(dir, "staging.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Config.Contexts["staging"]; ok {
		t.Error("context should be deleted")
	}
	if _, ok := f.Config.Clusters["staging-cluster"]; !ok {
		t.Error("cluster should survive with --keep-orphans")
	}
	if _, ok := f.Config.AuthInfos["staging-user"]; !ok {
		t.Error("user should survive with --keep-orphans")
	}
}

// -------- prune --------------------------------------------------------------

func TestPruneReportsOrphanedAndTopologyChangedEntries(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	// Create a legit state entry by enabling alerts on prod.
	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	// Inject an orphan entry (path_hint points to a non-existent file).
	storePath := filepath.Join(stateHome, "kubeconfig-manager", "config.yaml")
	store := state.NewFileStore(storePath)
	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.Entries["sha256:orphan"] = state.Entry{PathHint: "gone.yaml"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Inject a topology-mismatch entry: path_hint resolves to staging.yaml but
	// keyed under a hash that doesn't match its current stable fingerprint.
	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.Entries["sha256:stale"] = state.Entry{PathHint: "staging.yaml"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Dry-run prune should list both.
	out, _, err := runCmdInState(t, stateHome, "prune", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sha256:orphan") {
		t.Errorf("prune output missing orphan: %s", out)
	}
	if !strings.Contains(out, "sha256:stale") {
		t.Errorf("prune output missing stale topology entry: %s", out)
	}
	if !strings.Contains(out, "file not found") {
		t.Errorf("prune output missing 'file not found' reason: %s", out)
	}
	if !strings.Contains(out, "topology changed") {
		t.Errorf("prune output missing 'topology changed' reason: %s", out)
	}
	if !strings.Contains(out, "--yes") {
		t.Errorf("dry-run should suggest --yes: %s", out)
	}

	// Nothing removed yet.
	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Entries["sha256:orphan"]; !ok {
		t.Error("dry-run should not have removed the orphan")
	}

	// Actually prune.
	out, _, err = runCmdInState(t, stateHome, "prune", "--dir", dir, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Removed") {
		t.Errorf("prune --yes should confirm removal: %s", out)
	}
	cfg, err = store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Entries["sha256:orphan"]; ok {
		t.Error("orphan should be removed")
	}
	if _, ok := cfg.Entries["sha256:stale"]; ok {
		t.Error("stale topology entry should be removed")
	}
	// Legit prod entry should survive.
	if len(cfg.Entries) == 0 {
		t.Error("prune removed too much — legit entries gone")
	}
}

func TestPruneNoStaleEntriesReportsClean(t *testing.T) {
	dir := seedKubeconfigDir(t)
	stateHome := t.TempDir()

	if _, _, err := runCmdInState(t, stateHome, "alert", "enable", "prod", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmdInState(t, stateHome, "prune", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no stale state entries") {
		t.Errorf("expected clean report, got: %s", out)
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
