package audit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAppendAndTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	events := []Event{
		{Tool: "kubectl", Verb: "delete", Context: "prod-eu", Cluster: "prod-cluster", Decision: "approved"},
		{Tool: "helm", Verb: "upgrade", Context: "prod-eu", Decision: "declined", Severity: "hard", ValuePath: "test/values.yaml"},
		{Tool: "kubectl", Verb: "drain", Context: "prod-eu", Decision: "no-tty"},
	}
	for _, e := range events {
		if err := AppendAt(path, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	// Perms should be 0600 so a shared machine can't read past prompts.
	// Windows doesn't map POSIX bits cleanly onto ACLs, so skip there.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("audit file mode: got %o, want 0600", mode)
		}
	}

	// Tail returns lines in order; a large n returns all.
	all, err := Tail(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("tail returned %d lines, want 3", len(all))
	}
	if !strings.Contains(all[0], "tool=kubectl") || !strings.Contains(all[0], "verb=delete") {
		t.Errorf("first line: %s", all[0])
	}
	if !strings.Contains(all[1], "tool=helm") || !strings.Contains(all[1], "severity=hard") {
		t.Errorf("second line: %s", all[1])
	}

	// Partial tail returns the last n.
	last, err := Tail(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 2 || !strings.Contains(last[1], "decision=no-tty") {
		t.Errorf("tail(2): %v", last)
	}
}

func TestTailMissingFile(t *testing.T) {
	lines, err := Tail(filepath.Join(t.TempDir(), "never.log"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Errorf("expected empty slice for missing file, got %d lines", len(lines))
	}
}

func TestShellEscapeQuotesValuesWithSpaces(t *testing.T) {
	// Paths with spaces must round-trip the space without eating it at
	// grep-parse time. Locks the escape behavior for tests that grep the log.
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	if err := AppendAt(path, Event{
		Tool:      "helm",
		Verb:      "upgrade",
		ValuePath: "/a path/with spaces/values.yaml",
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "path='/a path/with spaces/values.yaml'") {
		t.Errorf("expected quoted path, got: %s", string(data))
	}
}
