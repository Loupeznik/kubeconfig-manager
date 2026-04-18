package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

func newStarshipCmd() *cobra.Command {
	var file string
	var contextName string

	cmd := &cobra.Command{
		Use:   "starship",
		Short: "Print a one-line tag/alert summary for starship's custom module",
		Long: `Prints a minimal summary of the active kubeconfig's tags and alert state,
suitable for consumption by starship's custom module. Exits silently with no
output when there is nothing worth showing — starship's 'when' predicate hides
the module in that case.

Output format:
  "⚠ prod,eu,critical"    — alerts enabled + tags
  "prod,eu"               — tags only
  "⚠"                     — alerts only
  ""                      — neither (module suppressed by starship)

Recommended starship config:

  [custom.kcm]
  command = "kubeconfig-manager starship"
  when = "kubeconfig-manager starship | grep -q ."
  shell = ["sh", "-c"]
  format = "[$output]($style) "
  style = "bold yellow"`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := buildStarshipLine(cmd.Context(), file, contextName)
			if out != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Kubeconfig path (default: first of $KUBECONFIG, else ~/.kube/config)")
	cmd.Flags().StringVar(&contextName, "context", "", "Context to summarize (default: the kubeconfig's current-context)")
	return cmd
}

// buildStarshipLine returns the one-line summary for starship, or "" to
// signal "nothing to show". Intentionally swallows errors — starship runs
// this on every prompt and must never produce noisy output.
func buildStarshipLine(ctx context.Context, file, contextName string) string {
	path := resolveStarshipPath(file)
	if path == "" {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}

	f, err := kubeconfig.Load(path)
	if err != nil {
		return ""
	}
	activeCtx := contextName
	if activeCtx == "" {
		activeCtx = f.Config.CurrentContext
	}

	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		return ""
	}
	store, err := state.DefaultStore()
	if err != nil {
		return ""
	}
	cfg, err := store.Load(ctx)
	if err != nil {
		return ""
	}
	entry, ok := cfg.GetEntry(id.StableHash, id.ContentHash)
	if !ok {
		return ""
	}

	alerts := entry.ResolveAlerts(activeCtx)
	tags := entry.ResolveTags(activeCtx)

	var parts []string
	if alerts.Enabled {
		parts = append(parts, "⚠")
	}
	if len(tags) > 0 {
		parts = append(parts, strings.Join(tags, ","))
	}
	return strings.Join(parts, " ")
}

func resolveStarshipPath(file string) string {
	if file != "" {
		return file
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		parts := strings.Split(env, string(os.PathListSeparator))
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	def, err := kubeconfig.DefaultPath()
	if err != nil {
		return ""
	}
	return def
}
