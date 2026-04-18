// Package audit appends a one-line record per guard prompt to a local audit
// log, and exposes readers for the `kcm audit` subcommand. The file lives at
// $XDG_DATA_HOME/kubeconfig-manager/audit.log with 0600 perms.
//
// The format is single-line key=value, chosen so `grep`, `awk`, and human eyes
// all work without a structured-log dependency. Example:
//
//	2026-04-18T15:12:47Z tool=kubectl verb=delete context=prod-eu cluster=prod-cluster decision=approved
//	2026-04-18T15:14:02Z tool=helm verb=upgrade context=prod-eu decision=aborted severity=hard path=test/values.yaml
package audit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
)

// DefaultPath returns the canonical audit log location. Callers can override
// via WithPath; the default routes through XDG_DATA_HOME so tests can isolate.
func DefaultPath() (string, error) {
	p, err := xdg.DataFile(filepath.Join("kubeconfig-manager", "audit.log"))
	if err != nil {
		return "", fmt.Errorf("resolve audit path: %w", err)
	}
	return p, nil
}

// Event captures one guard prompt outcome. All fields are optional; Append
// only writes the non-empty ones.
type Event struct {
	Tool      string // "kubectl" or "helm"
	Verb      string // e.g. "delete", "upgrade"
	Context   string // active kube context
	Cluster   string // active cluster (if resolved)
	Decision  string // "approved" | "declined" | "aborted" | "no-tty"
	Severity  string // helm-guard severity label, empty for kubectl
	ValuePath string // for helm: the values file that triggered the prompt
}

// Append writes one event line to the audit log. Errors are returned so the
// caller can decide whether to surface them — today the guard path just logs
// to stderr and keeps going, because a failed audit write shouldn't abort a
// command the user already approved.
func Append(e Event) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	return AppendAt(path, e)
}

// AppendAt is the path-explicit variant of Append; used by tests.
func AppendAt(path string, e Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(formatEvent(e))
	return err
}

// Tail returns the last n events from path, newest last. Missing file is
// treated as an empty log (not an error) — matches the `kcm audit` UX where
// "no prompts yet" is a normal state.
func Tail(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return tailLines(f, n)
}

func tailLines(r io.Reader, n int) ([]string, error) {
	sc := bufio.NewScanner(r)
	// Raise the per-line limit — audit lines are short, but guard paths could
	// be long absolute paths on monorepos.
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func formatEvent(e Event) string {
	var b strings.Builder
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	kv := func(k, v string) {
		if v == "" {
			return
		}
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(shellEscape(v))
	}
	kv("tool", e.Tool)
	kv("verb", e.Verb)
	kv("context", e.Context)
	kv("cluster", e.Cluster)
	kv("decision", e.Decision)
	kv("severity", e.Severity)
	kv("path", e.ValuePath)
	b.WriteByte('\n')
	return b.String()
}

// shellEscape quotes values that contain whitespace or = so greppers can parse
// the log with naive tokenizers. Uses single quotes with simple escaping since
// we only need round-trip for grep/awk, not eval.
func shellEscape(s string) string {
	if !strings.ContainsAny(s, " \t=\"'") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
