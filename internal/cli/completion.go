package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// completeKubeconfigNames lists kubeconfig filenames in the resolved --dir
// (or the default directory), offering both the full filename and the
// extension-stripped form so users can tab-complete `prod` → `prod.yaml`.
func completeKubeconfigNames(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	dir, _ := cmd.Flags().GetString("dir")
	resolvedDir, err := resolveDir(dir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	result, err := kubeconfig.ScanDir(resolvedDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(result.Files)*2)
	for _, f := range result.Files {
		base := f.Name()
		if !seen[base] {
			seen[base] = true
			out = append(out, base)
		}
		stripped := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
		if stripped != base && !seen[stripped] {
			seen[stripped] = true
			out = append(out, stripped)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeContextsForArgIdx returns a flag-completion function that reads the
// kubeconfig at args[argIdx] and enumerates its context names. Used for the
// --context flag on tag/alert subcommands whose first positional is the file.
func completeContextsForArgIdx(argIdx int) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if argIdx >= len(args) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		dir, _ := cmd.Flags().GetString("dir")
		resolvedDir, err := resolveDir(dir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		path, err := kubeconfig.ResolvePath(args[argIdx], resolvedDir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		f, err := kubeconfig.Load(path)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return f.ContextNames(), cobra.ShellCompDirectiveNoFileComp
	}
}

// completePaletteTags lists every tag currently in the palette — used for
// `kcm tag palette remove <TAB>`.
func completePaletteTags(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	store, err := state.DefaultStore()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	cfg, err := store.Load(context.Background())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	cfg.EnsurePaletteFromEntries()
	return append([]string(nil), cfg.AvailableTags...), cobra.ShellCompDirectiveNoFileComp
}
