package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

type staleEntry struct {
	key      string
	pathHint string
	reason   string
}

func newPruneCmd() *cobra.Command {
	var dir string
	var yes bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "List (and optionally remove) state entries whose kubeconfig is gone or whose topology changed",
		Long: "Walks the state file and reports entries that no longer match an actual\n" +
			"kubeconfig on disk — either the file referenced by path_hint is missing,\n" +
			"or its current stable fingerprint differs from the entry's key (meaning\n" +
			"the kubeconfig was edited in a way that changed its logical topology).\n\n" +
			"Default is a dry run: prints the list and exits. Pass --yes to actually\n" +
			"remove the stale entries.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			cfg, err := store.Load(ctx)
			if err != nil {
				return err
			}

			stale := findStaleEntries(cfg, resolvedDir)
			out := cmd.OutOrStdout()
			if len(stale) == 0 {
				_, _ = fmt.Fprintln(out, "no stale state entries found")
				return nil
			}

			_, _ = fmt.Fprintf(out, "Found %d stale entr%s:\n\n", len(stale), plural(len(stale), "y", "ies"))
			for _, s := range stale {
				hint := s.pathHint
				if hint == "" {
					hint = "(no path hint)"
				}
				_, _ = fmt.Fprintf(out, "  %s\n    path_hint: %s\n    reason: %s\n\n", s.key, hint, s.reason)
			}

			if !yes {
				_, _ = fmt.Fprintln(out, "Run `kcm prune --yes` to remove these entries.")
				return nil
			}

			if err := store.Mutate(ctx, func(cfg *state.Config) error {
				// Recompute on the fresh state loaded inside Mutate — the
				// filesystem snapshot we checked earlier may have drifted.
				victims := findStaleEntries(cfg, resolvedDir)
				for _, s := range victims {
					delete(cfg.Entries, s.key)
				}
				return nil
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "Removed %d stale entr%s.\n", len(stale), plural(len(stale), "y", "ies"))
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory to match path_hints against (default: ~/.kube)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Actually remove the stale entries (default: dry-run)")
	return cmd
}

// findStaleEntries walks cfg.Entries and returns the ones whose path_hint
// either doesn't resolve to a file in dir or whose current stable hash doesn't
// match the entry key. Entries without a path_hint are left alone because
// there's no way to verify them.
func findStaleEntries(cfg *state.Config, dir string) []staleEntry {
	var stale []staleEntry
	keys := make([]string, 0, len(cfg.Entries))
	for k := range cfg.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := cfg.Entries[key]
		if entry.PathHint == "" {
			continue
		}
		path := filepath.Join(dir, entry.PathHint)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			stale = append(stale, staleEntry{
				key:      key,
				pathHint: entry.PathHint,
				reason:   "file not found in " + dir,
			})
			continue
		}
		if err != nil || info.IsDir() {
			stale = append(stale, staleEntry{
				key:      key,
				pathHint: entry.PathHint,
				reason:   "cannot stat file: " + errMsg(err),
			})
			continue
		}
		id, err := kubeconfig.IdentifyFile(path)
		if err != nil {
			stale = append(stale, staleEntry{
				key:      key,
				pathHint: entry.PathHint,
				reason:   "cannot parse kubeconfig: " + err.Error(),
			})
			continue
		}
		if key == id.StableHash || key == id.ContentHash {
			// key matches current stable hash, or matches old content hash
			// (lazy migration will rekey on next mutation) — not stale.
			continue
		}
		stale = append(stale, staleEntry{
			key:      key,
			pathHint: entry.PathHint,
			reason:   "topology changed: file now hashes to " + id.StableHash,
		})
	}
	return stale
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

func errMsg(err error) string {
	if err == nil {
		return "is a directory"
	}
	return err.Error()
}
