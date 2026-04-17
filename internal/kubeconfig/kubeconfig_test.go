package kubeconfig

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const validKubeconfig = `apiVersion: v1
kind: Config
current-context: prod
clusters:
  - name: prod-cluster
    cluster:
      server: https://prod.example.com
  - name: staging-cluster
    cluster:
      server: https://staging.example.com
contexts:
  - name: prod
    context:
      cluster: prod-cluster
      user: prod-user
      namespace: default
  - name: staging
    context:
      cluster: staging-cluster
      user: staging-user
users:
  - name: prod-user
    user:
      token: redacted
  - name: staging-user
    user:
      token: redacted
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "prod.yaml", validKubeconfig)

	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(f.ContextNames()); got != 2 {
		t.Fatalf("contexts: got %d, want 2", got)
	}
	if f.Config.CurrentContext != "prod" {
		t.Errorf("current context: got %q, want prod", f.Config.CurrentContext)
	}
	if got := f.ClusterNames(); len(got) != 2 {
		t.Errorf("clusters: got %v", got)
	}
	if got := f.UserNames(); len(got) != 2 {
		t.Errorf("users: got %v", got)
	}
}

func TestLoadEmptyIsRejected(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "empty.yaml", "apiVersion: v1\nkind: Config\n")

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty kubeconfig")
	}
}

func TestLoadNonexistent(t *testing.T) {
	if _, err := Load("/nonexistent/path.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestScanDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "prod.yaml", validKubeconfig)
	writeFile(t, dir, "staging.yml", validKubeconfig)
	writeFile(t, dir, ".hidden.yaml", validKubeconfig)
	writeFile(t, dir, "broken.yaml", "not: [a: valid kubeconfig")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if got := len(result.Files); got != 2 {
		t.Errorf("files: got %d, want 2 (hidden + subdir + broken excluded)", got)
	}
	if got := len(result.Warnings); got != 1 {
		t.Errorf("warnings: got %d, want 1 (broken.yaml)", got)
	}

	names := []string{result.Files[0].Name(), result.Files[1].Name()}
	wantSet := map[string]bool{"prod.yaml": true, "staging.yml": true}
	for _, n := range names {
		if !wantSet[n] {
			t.Errorf("unexpected file in result: %s", n)
		}
	}
}

func TestScanDirMissing(t *testing.T) {
	if _, err := ScanDir("/nonexistent/dir"); err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "prod.yaml", validKubeconfig)

	got, err := ResolvePath(path, "/some/other/dir")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Errorf("got %s, want %s", got, path)
	}
}

func TestResolvePathBareNameWithYamlExt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "prod.yaml", validKubeconfig)

	got, err := ResolvePath("prod", dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "prod.yaml")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolvePathBareNameWithYmlExt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "staging.yml", validKubeconfig)

	got, err := ResolvePath("staging", dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "staging.yml")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolvePathExactFilename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config", validKubeconfig)

	got, err := ResolvePath("config", dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "config")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolvePathNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := ResolvePath("missing", dir)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got err %v, want ErrNotFound", err)
	}

	_, err = ResolvePath("/tmp/definitely/does/not/exist/config.yaml", dir)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got err %v, want ErrNotFound", err)
	}
}

func TestResolvePathTildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}
	scratch := filepath.Join(home, ".kcm-test-scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	writeFile(t, scratch, "prod.yaml", validKubeconfig)

	got, err := ResolvePath("~/.kcm-test-scratch/prod.yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(scratch, "prod.yaml")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
