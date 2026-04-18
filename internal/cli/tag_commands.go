package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// newTagCmd wires `kcm tag add|remove|list [palette]`. Tag assignments are
// validated against the global palette — unknown tags error out unless
// --allow-new auto-adds them to the palette.
func newTagCmd() *cobra.Command {
	var dir string
	var contextName string

	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags on kubeconfig files or individual contexts",
	}

	addCmd := &cobra.Command{
		Use:   "add <file> <tag...>",
		Short: "Add tags (file-level, or --context for one context only)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tags := args[0], args[1:]
			var added []string
			if err := mutateEntry(cmd, name, dir, func(_ string, e *state.Entry) error {
				if contextName != "" {
					added = e.AddContextTags(contextName, tags...)
				} else {
					added = e.AddTags(tags...)
				}
				return nil
			}); err != nil {
				return err
			}
			scope := "file"
			if contextName != "" {
				scope = "context " + contextName
			}
			if len(added) == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no new tags added (%s)\n", scope)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added tags (%s): %s\n", scope, strings.Join(added, ", "))
			}
			return nil
		},
	}
	removeCmd := &cobra.Command{
		Use:   "remove <file> <tag...>",
		Short: "Remove tags (file-level, or --context for one context only)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, tags := args[0], args[1:]
			var removed []string
			if err := mutateEntry(cmd, name, dir, func(_ string, e *state.Entry) error {
				if contextName != "" {
					removed = e.RemoveContextTags(contextName, tags...)
				} else {
					removed = e.RemoveTags(tags...)
				}
				return nil
			}); err != nil {
				return err
			}
			scope := "file"
			if contextName != "" {
				scope = "context " + contextName
			}
			if len(removed) == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no matching tags to remove (%s)\n", scope)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed tags (%s): %s\n", scope, strings.Join(removed, ", "))
			}
			return nil
		},
	}
	listCmd := &cobra.Command{
		Use:   "list [file]",
		Short: "List tags for one kubeconfig (with per-context rows) or all kubeconfigs",
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
				id, err := kubeconfig.IdentifyFile(path)
				if err != nil {
					return err
				}
				entry, _ := cfg.GetEntry(id.StableHash, id.ContentHash)
				if contextName != "" {
					tags := entry.ResolveTags(contextName)
					if len(tags) == 0 {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s [context %s]: no tags\n", filepath.Base(path), contextName)
						return nil
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s [context %s]: %s\n", filepath.Base(path), contextName, strings.Join(tags, ", "))
					return nil
				}
				printEntryTags(cmd.OutOrStdout(), filepath.Base(path), entry)
				return nil
			}

			result, err := kubeconfig.ScanDir(resolvedDir)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "FILE\tSCOPE\tTAGS")
			for _, f := range result.Files {
				id, err := kubeconfig.IdentifyFile(f.Path)
				if err != nil {
					return err
				}
				entry, _ := cfg.GetEntry(id.StableHash, id.ContentHash)
				fileTags := "-"
				if len(entry.Tags) > 0 {
					fileTags = strings.Join(entry.Tags, ", ")
				}
				_, _ = fmt.Fprintf(tw, "%s\tfile\t%s\n", f.Name(), fileTags)
				for _, ctxName := range sortedContextTagKeys(entry.ContextTags) {
					_, _ = fmt.Fprintf(tw, "%s\tctx:%s\t%s\n", f.Name(), ctxName, strings.Join(entry.ContextTags[ctxName], ", "))
				}
			}
			return tw.Flush()
		},
	}

	var allowNew bool
	addCmd.Flags().BoolVar(&allowNew, "allow-new", false, "Add tags to the palette automatically if not present")

	// Wrap addCmd.RunE with a palette validation preflight. If the palette is
	// empty we stay permissive (bootstrap flow), otherwise every tag must
	// already exist unless --allow-new was passed.
	origAddRun := addCmd.RunE
	addCmd.RunE = func(cmd *cobra.Command, args []string) error {
		store, err := state.DefaultStore()
		if err != nil {
			return err
		}
		ctx := cmd.Context()
		cfg, err := store.Load(ctx)
		if err != nil {
			return err
		}
		cfg.EnsurePaletteFromEntries()
		if len(cfg.AvailableTags) > 0 {
			var unknown []string
			for _, t := range args[1:] {
				if !cfg.IsTagInPalette(t) {
					unknown = append(unknown, t)
				}
			}
			if len(unknown) > 0 && !allowNew {
				return fmt.Errorf("tag(s) not in palette: %s (run 'kcm tag palette add %s' first, or pass --allow-new)",
					strings.Join(unknown, ", "), strings.Join(unknown, " "))
			}
			if len(unknown) > 0 && allowNew {
				added := cfg.AddAvailableTags(unknown...)
				if err := store.Save(ctx, cfg); err != nil {
					return fmt.Errorf("update palette: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added to palette: %s\n", strings.Join(added, ", "))
			}
		}
		return origAddRun(cmd, args)
	}

	for _, c := range []*cobra.Command{addCmd, removeCmd, listCmd} {
		c.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
		c.Flags().StringVar(&contextName, "context", "", "Apply to this context only (default: file-level)")
		c.ValidArgsFunction = completeKubeconfigNames
		_ = c.RegisterFlagCompletionFunc("context", completeContextsForArgIdx(0))
	}
	cmd.AddCommand(addCmd, removeCmd, listCmd, newTagPaletteCmd())
	return cmd
}

// newTagPaletteCmd manages the global allow-list of tag names. Removing a tag
// also scrubs it from every entry's file-level and per-context lists so the
// state stays internally consistent.
func newTagPaletteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "palette",
		Short:   "Manage the global tag palette (allowed tag set)",
		Aliases: []string{"palette", "tags"},
	}

	addCmd := &cobra.Command{
		Use:   "add <tag...>",
		Short: "Add tags to the global palette",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			var added []string
			if err := store.Mutate(cmd.Context(), func(cfg *state.Config) error {
				cfg.EnsurePaletteFromEntries()
				added = cfg.AddAvailableTags(args...)
				return nil
			}); err != nil {
				return err
			}
			if len(added) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no new tags added to palette")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added to palette: %s\n", strings.Join(added, ", "))
			}
			return nil
		},
	}

	removeCmd := &cobra.Command{
		Use:               "remove <tag...>",
		Short:             "Remove tags from the palette (also scrubs them from every entry)",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completePaletteTags,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			var removed []string
			if err := store.Mutate(cmd.Context(), func(cfg *state.Config) error {
				removed = cfg.RemoveAvailableTags(args...)
				return nil
			}); err != nil {
				return err
			}
			if len(removed) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no matching tags in palette")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed from palette (and scrubbed from entries): %s\n", strings.Join(removed, ", "))
			}
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Show the current palette",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			cfg, err := store.Load(cmd.Context())
			if err != nil {
				return err
			}
			cfg.EnsurePaletteFromEntries()
			if len(cfg.AvailableTags) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "palette is empty — add tags with 'kcm tag palette add <tag...>'")
				return nil
			}
			for _, t := range cfg.AvailableTags {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), t)
			}
			return nil
		},
	}

	cmd.AddCommand(addCmd, removeCmd, listCmd)
	return cmd
}

// printEntryTags renders file-level + per-context tag lists for a single entry.
func printEntryTags(out io.Writer, name string, entry state.Entry) {
	if len(entry.Tags) == 0 {
		_, _ = fmt.Fprintf(out, "%s [file]: no tags\n", name)
	} else {
		_, _ = fmt.Fprintf(out, "%s [file]: %s\n", name, strings.Join(entry.Tags, ", "))
	}
	for _, ctx := range sortedContextTagKeys(entry.ContextTags) {
		_, _ = fmt.Fprintf(out, "%s [context %s]: %s\n", name, ctx, strings.Join(entry.ContextTags[ctx], ", "))
	}
}

func sortedContextTagKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
