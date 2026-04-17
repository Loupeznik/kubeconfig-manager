package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

func newTagCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags on kubeconfig files",
	}

	addCmd := &cobra.Command{
		Use:   "add <file> <tag...>",
		Short: "Add tags to a kubeconfig",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tags := args[0], args[1:]
			var added []string
			if err := mutateEntry(cmd, name, dir, func(_ string, e *state.Entry) error {
				added = e.AddTags(tags...)
				return nil
			}); err != nil {
				return err
			}
			if len(added) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no new tags added")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added tags: %s\n", strings.Join(added, ", "))
			}
			return nil
		},
	}
	removeCmd := &cobra.Command{
		Use:   "remove <file> <tag...>",
		Short: "Remove tags from a kubeconfig",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tags := args[0], args[1:]
			var removed []string
			if err := mutateEntry(cmd, name, dir, func(_ string, e *state.Entry) error {
				removed = e.RemoveTags(tags...)
				return nil
			}); err != nil {
				return err
			}
			if len(removed) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no matching tags to remove")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed tags: %s\n", strings.Join(removed, ", "))
			}
			return nil
		},
	}
	listCmd := &cobra.Command{
		Use:   "list [file]",
		Short: "List tags for one kubeconfig or all kubeconfigs in the directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			cfg, err := store.Load(cmd.Context())
			if err != nil {
				return err
			}

			if len(args) == 1 {
				path, err := kubeconfig.ResolvePath(args[0], resolvedDir)
				if err != nil {
					return err
				}
				hash, err := kubeconfig.HashFile(path)
				if err != nil {
					return err
				}
				entry := cfg.Entries[hash]
				if len(entry.Tags) == 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: no tags\n", filepath.Base(path))
					return nil
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", filepath.Base(path), strings.Join(entry.Tags, ", "))
				return nil
			}

			result, err := kubeconfig.ScanDir(resolvedDir)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "FILE\tTAGS")
			for _, f := range result.Files {
				hash, err := kubeconfig.HashFile(f.Path)
				if err != nil {
					return err
				}
				entry := cfg.Entries[hash]
				tags := "-"
				if len(entry.Tags) > 0 {
					tags = strings.Join(entry.Tags, ", ")
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\n", f.Name(), tags)
			}
			return tw.Flush()
		},
	}

	for _, c := range []*cobra.Command{addCmd, removeCmd, listCmd} {
		c.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	}
	cmd.AddCommand(addCmd, removeCmd, listCmd)
	return cmd
}

func newAlertCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "alert",
		Short: "Configure destructive-action alerts per kubeconfig",
	}

	enableCmd := &cobra.Command{
		Use:   "enable <file>",
		Short: "Enable alerts for a kubeconfig (populates default blocked verbs)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := mutateEntry(cmd, args[0], dir, func(_ string, e *state.Entry) error {
				e.Alerts.Enabled = true
				if !e.Alerts.RequireConfirmation && !e.Alerts.ConfirmClusterName {
					e.Alerts.RequireConfirmation = true
				}
				if len(e.Alerts.BlockedVerbs) == 0 {
					e.Alerts.BlockedVerbs = state.DefaultBlockedVerbs()
				}
				return nil
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "alerts enabled")
			return nil
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable <file>",
		Short: "Disable alerts for a kubeconfig",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := mutateEntry(cmd, args[0], dir, func(_ string, e *state.Entry) error {
				e.Alerts.Enabled = false
				return nil
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "alerts disabled")
			return nil
		},
	}

	showCmd := &cobra.Command{
		Use:   "show <file>",
		Short: "Show alert policy for a kubeconfig",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			path, err := kubeconfig.ResolvePath(args[0], resolvedDir)
			if err != nil {
				return err
			}
			hash, err := kubeconfig.HashFile(path)
			if err != nil {
				return err
			}
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			cfg, err := store.Load(cmd.Context())
			if err != nil {
				return err
			}
			entry := cfg.Entries[hash]
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "File:                  %s\n", path)
			_, _ = fmt.Fprintf(out, "Enabled:               %t\n", entry.Alerts.Enabled)
			_, _ = fmt.Fprintf(out, "Require confirmation:  %t\n", entry.Alerts.RequireConfirmation)
			_, _ = fmt.Fprintf(out, "Confirm cluster name:  %t\n", entry.Alerts.ConfirmClusterName)
			verbs := entry.Alerts.BlockedVerbs
			if len(verbs) == 0 {
				verbs = state.DefaultBlockedVerbs()
			}
			_, _ = fmt.Fprintf(out, "Blocked verbs:         %s\n", strings.Join(verbs, ", "))
			return nil
		},
	}

	for _, c := range []*cobra.Command{enableCmd, disableCmd, showCmd} {
		c.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	}
	cmd.AddCommand(enableCmd, disableCmd, showCmd)
	return cmd
}

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

			hash, err := kubeconfig.HashFile(oldPath)
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
				entry, ok := cfg.Entries[hash]
				if !ok {
					return nil
				}
				entry.PathHint = filepath.Base(newPath)
				entry.Touch()
				cfg.Entries[hash] = entry
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
	return cmd
}

func mutateEntry(cmd *cobra.Command, nameOrPath, dir string, fn func(path string, e *state.Entry) error) error {
	resolvedDir, err := resolveDir(dir)
	if err != nil {
		return err
	}
	path, err := kubeconfig.ResolvePath(nameOrPath, resolvedDir)
	if err != nil {
		return err
	}
	hash, err := kubeconfig.HashFile(path)
	if err != nil {
		return err
	}
	store, err := state.DefaultStore()
	if err != nil {
		return err
	}
	return store.Mutate(cmd.Context(), func(cfg *state.Config) error {
		entry := cfg.Entries[hash]
		entry.PathHint = filepath.Base(path)
		if err := fn(path, &entry); err != nil {
			return err
		}
		entry.Touch()
		cfg.Entries[hash] = entry
		return nil
	})
}
