package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFlag(t *testing.T) {
	tests := []struct {
		in      string
		want    Shell
		wantErr bool
	}{
		{"", Unknown, false},
		{"bash", Bash, false},
		{"BASH", Bash, false},
		{"zsh", Zsh, false},
		{"pwsh", PowerShell, false},
		{"powershell", PowerShell, false},
		{"fish", Fish, false},
		{"tcsh", Unknown, true},
	}
	for _, tt := range tests {
		got, err := ParseFlag(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseFlag(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseFlag(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestExportLineBash(t *testing.T) {
	line, err := ExportLine(Bash, "/home/user/.kube/prod.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := `export KUBECONFIG='/home/user/.kube/prod.yaml'`
	if line != want {
		t.Errorf("got %q, want %q", line, want)
	}
}

func TestExportLineZsh(t *testing.T) {
	line, err := ExportLine(Zsh, "/home/user/.kube/prod.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "export KUBECONFIG=") {
		t.Errorf("zsh line: %q", line)
	}
}

func TestExportLinePowerShell(t *testing.T) {
	line, err := ExportLine(PowerShell, `C:\Users\dev\.kube\prod.yaml`)
	if err != nil {
		t.Fatal(err)
	}
	want := `$env:KUBECONFIG = 'C:\Users\dev\.kube\prod.yaml'`
	if line != want {
		t.Errorf("got %q, want %q", line, want)
	}
}

func TestExportLineEscapesApostrophes(t *testing.T) {
	bash, err := ExportLine(Bash, "/home/o'reilly/kube/prod.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bash, `'\''`) {
		t.Errorf("bash escape missing: %q", bash)
	}

	ps, err := ExportLine(PowerShell, "/home/o'reilly/kube/prod.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ps, "o''reilly") {
		t.Errorf("pwsh escape missing: %q", ps)
	}
}

func TestExportLineFish(t *testing.T) {
	line, err := ExportLine(Fish, "/home/user/.kube/prod.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := `set -gx KUBECONFIG '/home/user/.kube/prod.yaml'`
	if line != want {
		t.Errorf("got %q, want %q", line, want)
	}
}

func TestFishQuoteEscapesBackslashAndQuote(t *testing.T) {
	line, err := ExportLine(Fish, `/home/o'reilly/path\with\backslash`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, `\'`) {
		t.Errorf("fish apostrophe escape missing: %q", line)
	}
	if !strings.Contains(line, `\\`) {
		t.Errorf("fish backslash escape missing: %q", line)
	}
}

func TestRenderHookFish(t *testing.T) {
	hook, err := RenderHook(Fish, HookOptions{AliasKubectl: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hook, "function kcm") {
		t.Errorf("fish hook missing kcm function: %s", hook)
	}
	if !strings.Contains(hook, "--shell=fish") {
		t.Errorf("fish hook missing shell flag: %s", hook)
	}
	if !strings.Contains(hook, `alias kubectl "command kubeconfig-manager kubectl"`) {
		t.Errorf("fish hook missing kubectl alias: %s", hook)
	}
}

func TestExportLineUnknownShell(t *testing.T) {
	if _, err := ExportLine(Unknown, "/x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderHookBashContainsKcmFunction(t *testing.T) {
	hook, err := RenderHook(Bash, HookOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hook, "kcm() {") {
		t.Errorf("hook missing kcm function: %s", hook)
	}
	if !strings.Contains(hook, "--shell=bash") {
		t.Errorf("hook missing bash shell flag: %s", hook)
	}
	if strings.Contains(hook, "alias kubectl=") {
		t.Errorf("hook contains kubectl alias without opt-in: %s", hook)
	}
}

func TestRenderHookBashWithKubectlAlias(t *testing.T) {
	hook, err := RenderHook(Bash, HookOptions{AliasKubectl: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hook, "alias kubectl='command kubeconfig-manager kubectl'") {
		t.Errorf("hook missing kubectl alias: %s", hook)
	}
}

func TestRenderHookPwsh(t *testing.T) {
	hook, err := RenderHook(PowerShell, HookOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hook, "function kcm {") {
		t.Errorf("pwsh hook missing kcm function: %s", hook)
	}
	if !strings.Contains(hook, "Invoke-Expression") {
		t.Errorf("pwsh hook missing Invoke-Expression: %s", hook)
	}
}

func TestInstallHookCreatesFile(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, "subdir", ".bashrc")
	hook, _ := RenderHook(Bash, HookOptions{})

	res, err := InstallHook(rc, hook)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created {
		t.Error("expected Created=true for new file")
	}
	content, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), fenceStart) || !strings.Contains(string(content), fenceEnd) {
		t.Errorf("fence markers missing: %s", content)
	}
}

func TestInstallHookPreservesExistingContent(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	existing := "export PATH=/custom:$PATH\n# some comment\n"
	if err := os.WriteFile(rc, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	hook, _ := RenderHook(Bash, HookOptions{})

	_, err := InstallHook(rc, hook)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	if !strings.Contains(gotStr, "export PATH=/custom:$PATH") {
		t.Errorf("pre-existing content lost: %s", gotStr)
	}
	if !strings.Contains(gotStr, fenceStart) {
		t.Errorf("hook not installed: %s", gotStr)
	}
}

func TestInstallHookReplacesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	oldHook, _ := RenderHook(Bash, HookOptions{})
	existing := "# user content\n" + oldHook + "# more user content\n"
	if err := os.WriteFile(rc, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	newHook, _ := RenderHook(Bash, HookOptions{AliasKubectl: true})
	res, err := InstallHook(rc, newHook)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated {
		t.Error("expected Updated=true when replacing block")
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	if !strings.Contains(gotStr, "alias kubectl=") {
		t.Errorf("new hook (with alias) not installed: %s", gotStr)
	}
	if !strings.Contains(gotStr, "# user content") || !strings.Contains(gotStr, "# more user content") {
		t.Errorf("user content lost: %s", gotStr)
	}
	if strings.Count(gotStr, fenceStart) != 1 {
		t.Errorf("expected exactly one fence block, got %d", strings.Count(gotStr, fenceStart))
	}
}

func TestUninstallHookRemovesBlock(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	hook, _ := RenderHook(Bash, HookOptions{})
	content := "# before\n" + hook + "# after\n"
	if err := os.WriteFile(rc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := UninstallHook(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	if strings.Contains(gotStr, fenceStart) {
		t.Errorf("fence still present: %s", gotStr)
	}
	if !strings.Contains(gotStr, "# before") || !strings.Contains(gotStr, "# after") {
		t.Errorf("user content lost: %s", gotStr)
	}
}

func TestUninstallHookNoBlockIsNoop(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	if err := os.WriteFile(rc, []byte("export PATH=/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := UninstallHook(rc)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("expected removed=false")
	}
}

func TestUninstallHookMissingFileIsNoop(t *testing.T) {
	removed, err := UninstallHook(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("expected removed=false")
	}
}

func TestInstallHookIdempotent(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	hook, _ := RenderHook(Bash, HookOptions{})

	_, err := InstallHook(rc, hook)
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}

	_, err = InstallHook(rc, hook)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("second install changed file:\n=== first ===\n%s\n=== second ===\n%s", first, second)
	}
}
