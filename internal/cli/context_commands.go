package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Rename or delete contexts within a kubeconfig file",
	}
	cmd.AddCommand(newContextRenameCmd(), newContextDeleteCmd())
	return cmd
}

func newContextRenameCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "rename <file> <old-name> <new-name>",
		Short: "Rename a context and move its per-context state (tags, alerts) to the new name",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			path, err := kubeconfig.ResolvePath(args[0], resolvedDir)
			if err != nil {
				return err
			}
			oldName, newName := args[1], args[2]

			// Snapshot the identity BEFORE rewriting the file — the stable
			// hash will change once the context is renamed, and we need the
			// pre-rename hash to find the existing state entry.
			oldID, err := kubeconfig.IdentifyFile(path)
			if err != nil {
				return err
			}

			cfg, err := clientcmd.LoadFromFile(path)
			if err != nil {
				return fmt.Errorf("load %s: %w", path, err)
			}
			updated, err := kubeconfig.RenameContext(cfg, oldName, newName)
			if err != nil {
				return err
			}
			if err := clientcmd.WriteToFile(*updated, path); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}

			newID, err := kubeconfig.IdentifyFile(path)
			if err != nil {
				return err
			}

			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			if err := store.Mutate(cmd.Context(), func(cfg *state.Config) error {
				entry := cfg.TakeEntry(oldID.StableHash, oldID.ContentHash)
				if entry.ContextAlerts != nil {
					if a, ok := entry.ContextAlerts[oldName]; ok {
						delete(entry.ContextAlerts, oldName)
						entry.ContextAlerts[newName] = a
					}
				}
				if entry.ContextTags != nil {
					if t, ok := entry.ContextTags[oldName]; ok {
						delete(entry.ContextTags, oldName)
						entry.ContextTags[newName] = t
					}
				}
				entry.PathHint = filepath.Base(path)
				entry.Touch()
				cfg.Entries[newID.StableHash] = entry
				return nil
			}); err != nil {
				return fmt.Errorf("rename ok but state update failed: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "renamed context %q -> %q in %s\n", oldName, newName, path)
			return nil
		},
		ValidArgsFunction: func(c *cobra.Command, args []string, tc string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completeKubeconfigNames(c, args, tc)
			case 1:
				return completeContextsForArgIdx(0)(c, args, tc)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	return cmd
}

func newContextDeleteCmd() *cobra.Command {
	var dir string
	var keepOrphans bool
	cmd := &cobra.Command{
		Use:     "delete <file> <name>",
		Aliases: []string{"remove", "rm"},
		Short:   "Delete a context (and its per-context tags/alerts) from a kubeconfig",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			path, err := kubeconfig.ResolvePath(args[0], resolvedDir)
			if err != nil {
				return err
			}
			name := args[1]

			oldID, err := kubeconfig.IdentifyFile(path)
			if err != nil {
				return err
			}

			cfg, err := clientcmd.LoadFromFile(path)
			if err != nil {
				return fmt.Errorf("load %s: %w", path, err)
			}
			var updated *clientcmdapi.Config
			if keepOrphans {
				if _, ok := cfg.Contexts[name]; !ok {
					return fmt.Errorf("context %q not found", name)
				}
				updated = cloneForMinimalDelete(cfg, name)
			} else {
				updated, err = kubeconfig.Remove(cfg, name)
				if err != nil {
					return err
				}
			}
			if err := clientcmd.WriteToFile(*updated, path); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}

			newID, err := kubeconfig.IdentifyFile(path)
			if err != nil {
				return err
			}
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			if err := store.Mutate(cmd.Context(), func(cfg *state.Config) error {
				entry := cfg.TakeEntry(oldID.StableHash, oldID.ContentHash)
				delete(entry.ContextAlerts, name)
				delete(entry.ContextTags, name)
				entry.PathHint = filepath.Base(path)
				entry.Touch()
				cfg.Entries[newID.StableHash] = entry
				return nil
			}); err != nil {
				return fmt.Errorf("delete ok but state update failed: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "deleted context %q from %s\n", name, path)
			return nil
		},
		ValidArgsFunction: func(c *cobra.Command, args []string, tc string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completeKubeconfigNames(c, args, tc)
			case 1:
				return completeContextsForArgIdx(0)(c, args, tc)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().BoolVar(&keepOrphans, "keep-orphans", false, "Keep the referenced cluster/user even if no other context uses them")
	return cmd
}

// cloneForMinimalDelete drops only the named context (leaving cluster/user
// orphans in place) when --keep-orphans is passed.
func cloneForMinimalDelete(src *clientcmdapi.Config, name string) *clientcmdapi.Config {
	out := &clientcmdapi.Config{
		APIVersion:     src.APIVersion,
		Kind:           src.Kind,
		CurrentContext: src.CurrentContext,
		Preferences:    *src.Preferences.DeepCopy(),
		Clusters:       map[string]*clientcmdapi.Cluster{},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{},
		Contexts:       map[string]*clientcmdapi.Context{},
	}
	for k, v := range src.Clusters {
		out.Clusters[k] = v.DeepCopy()
	}
	for k, v := range src.AuthInfos {
		out.AuthInfos[k] = v.DeepCopy()
	}
	for k, v := range src.Contexts {
		if k == name {
			continue
		}
		out.Contexts[k] = v.DeepCopy()
	}
	if out.CurrentContext == name {
		out.CurrentContext = ""
	}
	return out
}
