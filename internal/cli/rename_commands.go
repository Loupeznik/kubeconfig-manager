package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// newRenameCmd renames a kubeconfig file on disk and rebinds the state
// entry's path_hint. Metadata follows the file because state is keyed by
// stable topology hash, not filename.
func newRenameCmd() *cobra.Command {
	var dir string
	var force bool

	cmd := &cobra.Command{
		Use:   "rename <file> <new-name>",
		Short: "Rename a kubeconfig file on disk (metadata re-binds automatically)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			oldPath, err := kubeconfig.ResolvePath(args[0], resolvedDir)
			if err != nil {
				return err
			}

			newName := args[1]
			if strings.ContainsRune(newName, os.PathSeparator) {
				return fmt.Errorf("new name %q must not contain path separators", newName)
			}
			newPath := filepath.Join(filepath.Dir(oldPath), newName)
			if oldPath == newPath {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "name unchanged")
				return nil
			}
			if _, err := os.Stat(newPath); err == nil && !force {
				return fmt.Errorf("destination %s already exists (pass --force to overwrite)", newPath)
			}

			id, err := kubeconfig.IdentifyFile(oldPath)
			if err != nil {
				return err
			}
			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("rename: %w", err)
			}

			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			if err := store.Mutate(cmd.Context(), func(cfg *state.Config) error {
				entry, ok := cfg.GetEntry(id.StableHash, id.ContentHash)
				if !ok {
					return nil
				}
				entry = cfg.TakeEntry(id.StableHash, id.ContentHash)
				entry.PathHint = filepath.Base(newPath)
				entry.Touch()
				cfg.Entries[id.StableHash] = entry
				return nil
			}); err != nil {
				return fmt.Errorf("rename file succeeded but state update failed: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "renamed %s -> %s\n", filepath.Base(oldPath), filepath.Base(newPath))
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite destination if it exists")
	cmd.ValidArgsFunction = func(c *cobra.Command, args []string, tc string) ([]string, cobra.ShellCompDirective) {
		// Only the first positional (the source kubeconfig) is completable;
		// the second positional is a free-form new filename.
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeKubeconfigNames(c, args, tc)
	}
	return cmd
}
