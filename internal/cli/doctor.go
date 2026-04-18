package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/shell"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// newDoctorCmd wires `kcm doctor` — an all-in-one diagnostic that walks the
// common kcm + kubectl + helm setup and reports anything amiss. Exits 1 if any
// check fails (warnings don't fail the exit code).
func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks against the local kcm + kubectl setup",
		Long: "Walks through the common misconfigurations in the order they bite users: " +
			"kubectl/helm on PATH, shell hook installed, state file schema, active " +
			"kubeconfig resolved, palette populated, stale state entries. Exits non-zero " +
			"if any check fails (warnings alone do not fail the exit code).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			results := runDoctorChecks(cmd.Context())
			anyFail := false
			for _, r := range results {
				_, _ = fmt.Fprintln(out, r.Render())
				if r.Level == doctorFail {
					anyFail = true
				}
			}
			if anyFail {
				return errors.New("doctor reported failures")
			}
			return nil
		},
	}
	return cmd
}

type doctorLevel int

const (
	doctorOK doctorLevel = iota
	doctorWarn
	doctorFail
)

type doctorResult struct {
	Name   string
	Level  doctorLevel
	Detail string
	Hint   string
}

func (r doctorResult) Render() string {
	icon := "[ok]"
	switch r.Level {
	case doctorWarn:
		icon = "[warn]"
	case doctorFail:
		icon = "[fail]"
	}
	line := fmt.Sprintf("%-6s %s — %s", icon, r.Name, r.Detail)
	if r.Hint != "" {
		line += "\n       hint: " + r.Hint
	}
	return line
}

// runDoctorChecks runs every check in order and returns their results. Order
// matches the review: what the user hits first when setting up kcm.
func runDoctorChecks(ctx context.Context) []doctorResult {
	var out []doctorResult
	out = append(out, checkBinaryOnPath("kubectl"))
	out = append(out, checkBinaryOnPath("helm"))
	out = append(out, checkShellHook())

	cfg, stateRes := checkStateFile(ctx)
	out = append(out, stateRes)

	out = append(out, checkActiveKubeconfig())
	out = append(out, checkPalettePopulated(cfg))
	out = append(out, checkStaleEntries(cfg))
	return out
}

func checkBinaryOnPath(name string) doctorResult {
	path, err := exec.LookPath(name)
	if err != nil {
		lvl := doctorWarn
		if name == "kubectl" {
			lvl = doctorFail
		}
		return doctorResult{
			Name:   name + " on PATH",
			Level:  lvl,
			Detail: "not found",
			Hint:   "install " + name + " and make sure it's on your PATH",
		}
	}
	version := firstLine(runAndCollect(name, "version", "--client=true", "--short"))
	if version == "" {
		version = firstLine(runAndCollect(name, "version", "--client=true"))
	}
	if version == "" {
		version = path
	}
	return doctorResult{
		Name:   name + " on PATH",
		Level:  doctorOK,
		Detail: version,
	}
}

func checkShellHook() doctorResult {
	sh := shell.Detect()
	rc, err := shell.RCPath(sh)
	if err != nil || rc == "" {
		return doctorResult{
			Name:   "shell hook",
			Level:  doctorWarn,
			Detail: "could not determine rc file for shell " + sh.String(),
			Hint:   "run 'kcm install-shell-hook --shell=<bash|zsh|fish|pwsh>' manually",
		}
	}
	data, err := os.ReadFile(rc)
	if errors.Is(err, os.ErrNotExist) {
		return doctorResult{
			Name:   "shell hook",
			Level:  doctorWarn,
			Detail: "no rc file at " + rc,
			Hint:   "run 'kcm install-shell-hook' to create it",
		}
	}
	if err != nil {
		return doctorResult{
			Name:   "shell hook",
			Level:  doctorWarn,
			Detail: "read " + rc + ": " + err.Error(),
		}
	}
	text := string(data)
	if !strings.Contains(text, "kubeconfig-manager shell hook") {
		return doctorResult{
			Name:   "shell hook",
			Level:  doctorWarn,
			Detail: "hook block not present in " + rc,
			Hint:   "run 'kcm install-shell-hook' (add --alias-kubectl / --alias-helm for guarded wrappers)",
		}
	}
	parts := []string{"installed"}
	if strings.Contains(text, "alias kubectl=") || strings.Contains(text, "alias kubectl ") {
		parts = append(parts, "alias-kubectl")
	}
	if strings.Contains(text, "alias helm=") || strings.Contains(text, "alias helm ") {
		parts = append(parts, "alias-helm")
	}
	return doctorResult{
		Name:   "shell hook",
		Level:  doctorOK,
		Detail: strings.Join(parts, " + ") + " (" + rc + ")",
	}
}

func checkStateFile(ctx context.Context) (*state.Config, doctorResult) {
	store, err := state.DefaultStore()
	if err != nil {
		return nil, doctorResult{
			Name:   "state file",
			Level:  doctorFail,
			Detail: "resolve path: " + err.Error(),
		}
	}
	cfg, err := store.Load(ctx)
	if err != nil {
		return nil, doctorResult{
			Name:   "state file",
			Level:  doctorFail,
			Detail: err.Error(),
			Hint:   "inspect " + store.Path() + " by hand; kcm won't mutate a file it can't parse",
		}
	}
	return cfg, doctorResult{
		Name:   "state file",
		Level:  doctorOK,
		Detail: fmt.Sprintf("v%d at %s (%d entr%s)", cfg.Version, store.Path(), len(cfg.Entries), plural(len(cfg.Entries), "y", "ies")),
	}
}

func checkActiveKubeconfig() doctorResult {
	paths := resolveKubeconfigPaths(os.Getenv("KUBECONFIG"))
	if len(paths) == 0 {
		return doctorResult{
			Name:   "active kubeconfig",
			Level:  doctorWarn,
			Detail: "neither $KUBECONFIG nor ~/.kube/config resolved",
		}
	}
	var missing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return doctorResult{
			Name:   "active kubeconfig",
			Level:  doctorWarn,
			Detail: "missing file(s): " + strings.Join(missing, ", "),
			Hint:   "remove the dead path from $KUBECONFIG",
		}
	}
	// Try to load the first path and surface the current context.
	first := paths[0]
	f, err := kubeconfig.Load(first)
	if err != nil {
		return doctorResult{
			Name:   "active kubeconfig",
			Level:  doctorWarn,
			Detail: "load " + first + ": " + err.Error(),
		}
	}
	ctx := f.Config.CurrentContext
	if ctx == "" {
		ctx = "(no current-context)"
	}
	return doctorResult{
		Name:   "active kubeconfig",
		Level:  doctorOK,
		Detail: fmt.Sprintf("%s · current: %s", first, ctx),
	}
}

func checkPalettePopulated(cfg *state.Config) doctorResult {
	if cfg == nil {
		return doctorResult{
			Name:   "tag palette",
			Level:  doctorWarn,
			Detail: "state file not loaded",
		}
	}
	if len(cfg.AvailableTags) == 0 {
		return doctorResult{
			Name:   "tag palette",
			Level:  doctorWarn,
			Detail: "empty",
			Hint:   "populate with 'kcm tag palette add <tag> [<tag> ...]' for the multi-select picker",
		}
	}
	return doctorResult{
		Name:   "tag palette",
		Level:  doctorOK,
		Detail: fmt.Sprintf("%d tag(s): %s", len(cfg.AvailableTags), strings.Join(cfg.AvailableTags, ", ")),
	}
}

func checkStaleEntries(cfg *state.Config) doctorResult {
	if cfg == nil {
		return doctorResult{
			Name:   "stale state entries",
			Level:  doctorWarn,
			Detail: "state file not loaded",
		}
	}
	dir, err := resolveDir("")
	if err != nil {
		return doctorResult{
			Name:   "stale state entries",
			Level:  doctorWarn,
			Detail: "resolve kubeconfig dir: " + err.Error(),
		}
	}
	stale := findStaleEntries(cfg, dir)
	if len(stale) == 0 {
		return doctorResult{
			Name:   "stale state entries",
			Level:  doctorOK,
			Detail: "none",
		}
	}
	hints := make([]string, 0, len(stale))
	for _, s := range stale {
		hint := s.pathHint
		if hint == "" {
			hint = "(no hint)"
		}
		hints = append(hints, hint)
	}
	sort.Strings(hints)
	return doctorResult{
		Name:   "stale state entries",
		Level:  doctorWarn,
		Detail: fmt.Sprintf("%d entr%s: %s", len(stale), plural(len(stale), "y", "ies"), strings.Join(hints, ", ")),
		Hint:   "run 'kcm prune --yes' to clean them up",
	}
}

// runAndCollect executes name with args and returns combined stdout trimmed.
// Errors are swallowed — doctor is best-effort and shouldn't surface nested
// errors for optional detail like version strings.
func runAndCollect(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// resolveKubeconfigPaths honors $KUBECONFIG's path-list shape (colon or
// semicolon separated) and falls back to kubeconfig.DefaultPath when the env
// var is unset or empty.
func resolveKubeconfigPaths(env string) []string {
	if strings.TrimSpace(env) == "" {
		def, err := kubeconfig.DefaultPath()
		if err != nil {
			return nil
		}
		return []string{def}
	}
	parts := strings.Split(env, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}
