package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// mutateEntry resolves a kubeconfig-name-or-path against the --dir flag, loads
// its identity, and runs fn inside a state.Store.Mutate callback with the
// entry-under-construction. Used by tag/alert/rename/helm-guard commands that
// all follow the same pattern: look up the file, take its entry (with lazy
// legacy-key migration), apply the caller's change, touch, write back.
func mutateEntry(cmd *cobra.Command, nameOrPath, dir string, fn func(path string, e *state.Entry) error) error {
	resolvedDir, err := resolveDir(dir)
	if err != nil {
		return err
	}
	path, err := kubeconfig.ResolvePath(nameOrPath, resolvedDir)
	if err != nil {
		return err
	}
	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		return err
	}
	store, err := state.DefaultStore()
	if err != nil {
		return err
	}
	return store.Mutate(cmd.Context(), func(cfg *state.Config) error {
		entry := cfg.TakeEntry(id.StableHash, id.ContentHash)
		entry.PathHint = filepath.Base(path)
		if err := fn(path, &entry); err != nil {
			return err
		}
		entry.Touch()
		cfg.Entries[id.StableHash] = entry
		return nil
	})
}
