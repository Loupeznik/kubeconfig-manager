package guard

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubKubectl drops a shell-script kubectl at a temp path and prepends its
// directory to PATH for the duration of the test. The script body is whatever
// the test supplies — the usual `echo "$@"; exit N` pattern.
func stubKubectl(t *testing.T, script string) { stubBinary(t, "kubectl", script) }

// stubHelm mirrors stubKubectl for the helm binary.
func stubHelm(t *testing.T, script string) { stubBinary(t, "helm", script) }

func stubBinary(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skipf("stub %s uses /bin/sh; skip on Windows", name)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\n" + script
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExecReturnsZeroOnSuccess(t *testing.T) {
	stubKubectl(t, `echo ok; exit 0
`)
	var stdout, stderr bytes.Buffer
	code, err := Exec([]string{"get", "pods"}, ExecOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if code != 0 {
		t.Errorf("code: got %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Errorf("stdout: %q", stdout.String())
	}
}

func TestExecPropagatesExitCode(t *testing.T) {
	stubKubectl(t, `exit 7
`)
	code, err := Exec([]string{"delete", "pod", "x"}, ExecOptions{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if code != 7 {
		t.Errorf("code: got %d, want 7", code)
	}
}

func TestExecPassesArgsToKubectl(t *testing.T) {
	stubKubectl(t, `echo "args:" "$@"
`)
	var stdout bytes.Buffer
	_, err := Exec([]string{"delete", "ns", "test", "--wait=false"}, ExecOptions{
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	want := "args: delete ns test --wait=false"
	if got != want {
		t.Errorf("stub saw: %q; want %q", got, want)
	}
}

func TestExecPipesStderrSeparately(t *testing.T) {
	stubKubectl(t, `echo stdout-line; echo stderr-line 1>&2; exit 0
`)
	var stdout, stderr bytes.Buffer
	_, err := Exec([]string{"get", "pods"}, ExecOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(stdout.String(), "stdout-line") {
		t.Errorf("stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stderr-line") {
		t.Errorf("stderr: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "stderr-line") {
		t.Errorf("stderr leaked into stdout: %q", stdout.String())
	}
}

func TestExecErrorWhenKubectlMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH manipulation differs on Windows")
	}
	t.Setenv("PATH", "/var/empty-dir-that-does-not-exist")
	_, err := Exec([]string{"get", "pods"}, ExecOptions{
		Stdout: &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected error when kubectl is not on PATH")
	}
	if !strings.Contains(err.Error(), "kubectl not found") {
		t.Errorf("error should mention kubectl: %v", err)
	}
}

func TestExecEnvOverrideReachesChild(t *testing.T) {
	stubKubectl(t, `echo "KCM_TEST_VAR=$KCM_TEST_VAR"
`)
	var stdout bytes.Buffer
	_, err := Exec([]string{"version"}, ExecOptions{
		Stdout: &stdout,
		Env:    append(os.Environ(), "KCM_TEST_VAR=hello"),
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(stdout.String(), "KCM_TEST_VAR=hello") {
		t.Errorf("env not propagated: %q", stdout.String())
	}
}

// ---- ExecHelm -------------------------------------------------------------

func TestExecHelmReturnsZeroOnSuccess(t *testing.T) {
	stubHelm(t, `echo ok; exit 0
`)
	var stdout, stderr bytes.Buffer
	code, err := ExecHelm([]string{"upgrade", "myapp"}, ExecOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if code != 0 {
		t.Errorf("code: got %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Errorf("stdout: %q", stdout.String())
	}
}

func TestExecHelmPropagatesExitCode(t *testing.T) {
	stubHelm(t, `exit 3
`)
	code, err := ExecHelm([]string{"upgrade", "broken"}, ExecOptions{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if code != 3 {
		t.Errorf("code: got %d, want 3", code)
	}
}

func TestExecHelmPassesArgsThrough(t *testing.T) {
	stubHelm(t, `echo "args:" "$@"
`)
	var stdout bytes.Buffer
	_, err := ExecHelm(
		[]string{"upgrade", "myapp", "-f", "clusters/prod/values.yaml", "--atomic"},
		ExecOptions{Stdout: &stdout},
	)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	want := "args: upgrade myapp -f clusters/prod/values.yaml --atomic"
	if got != want {
		t.Errorf("stub saw: %q; want %q", got, want)
	}
}

func TestExecHelmPipesStderrSeparately(t *testing.T) {
	stubHelm(t, `echo stdout-line; echo stderr-line 1>&2; exit 0
`)
	var stdout, stderr bytes.Buffer
	_, err := ExecHelm([]string{"list"}, ExecOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(stdout.String(), "stdout-line") {
		t.Errorf("stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stderr-line") {
		t.Errorf("stderr: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "stderr-line") {
		t.Errorf("stderr leaked into stdout: %q", stdout.String())
	}
}

func TestExecHelmErrorWhenHelmMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH manipulation differs on Windows")
	}
	t.Setenv("PATH", "/var/empty-dir-that-does-not-exist")
	_, err := ExecHelm([]string{"version"}, ExecOptions{
		Stdout: &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected error when helm is not on PATH")
	}
	if !strings.Contains(err.Error(), "helm not found") {
		t.Errorf("error should mention helm: %v", err)
	}
}
