package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// newAlertCmd wires the `kcm alert enable|disable|show` tree.
//
// Policy resolution order at call time: per-context override > file-level > off.
// Enable populates sensible defaults (RequireConfirmation=true, standard
// BlockedVerbs) so users don't have to hand-craft the first policy.
func newAlertCmd() *cobra.Command {
	var dir string
	var contextName string

	cmd := &cobra.Command{
		Use:   "alert",
		Short: "Configure destructive-action alerts per kubeconfig or context",
	}

	enableCmd := &cobra.Command{
		Use:   "enable <file>",
		Short: "Enable alerts (file-level, or --context for one context only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := mutateEntry(cmd, args[0], dir, func(_ string, e *state.Entry) error {
				if contextName != "" {
					if e.ContextAlerts == nil {
						e.ContextAlerts = map[string]state.Alerts{}
					}
					a := e.ContextAlerts[contextName]
					a.Enabled = true
					if !a.RequireConfirmation && !a.ConfirmClusterName {
						a.RequireConfirmation = true
					}
					if len(a.BlockedVerbs) == 0 {
						a.BlockedVerbs = state.DefaultBlockedVerbs()
					}
					e.ContextAlerts[contextName] = a
					return nil
				}
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
			if contextName != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "alerts enabled for context %s\n", contextName)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "alerts enabled (file-level)")
			}
			return nil
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable <file>",
		Short: "Disable alerts (file-level, or --context for one context only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := mutateEntry(cmd, args[0], dir, func(_ string, e *state.Entry) error {
				if contextName != "" {
					if e.ContextAlerts == nil {
						e.ContextAlerts = map[string]state.Alerts{}
					}
					a := e.ContextAlerts[contextName]
					a.Enabled = false
					e.ContextAlerts[contextName] = a
					return nil
				}
				e.Alerts.Enabled = false
				return nil
			}); err != nil {
				return err
			}
			if contextName != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "alerts disabled for context %s\n", contextName)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "alerts disabled (file-level)")
			}
			return nil
		},
	}

	showCmd := &cobra.Command{
		Use:   "show <file>",
		Short: "Show alert policy (file-level, per-context, or --context to filter)",
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
			id, err := kubeconfig.IdentifyFile(path)
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
			entry, _ := cfg.GetEntry(id.StableHash, id.ContentHash)
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "File: %s\n", path)

			if contextName != "" {
				printAlerts(out, "Context "+contextName, entry.ResolveAlerts(contextName))
				if _, ok := entry.ContextAlerts[contextName]; !ok {
					_, _ = fmt.Fprintf(out, "(no per-context override; inherits file-level policy)\n")
				}
				return nil
			}

			printAlerts(out, "File-level", entry.Alerts)
			if len(entry.ContextAlerts) > 0 {
				_, _ = fmt.Fprintln(out, "")
				_, _ = fmt.Fprintln(out, "Per-context overrides:")
				for _, name := range sortedAlertKeys(entry.ContextAlerts) {
					printAlerts(out, "  "+name, entry.ContextAlerts[name])
				}
			}
			return nil
		},
	}

	for _, c := range []*cobra.Command{enableCmd, disableCmd, showCmd} {
		c.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
		c.Flags().StringVar(&contextName, "context", "", "Apply to this context only (default: file-level)")
		c.ValidArgsFunction = completeKubeconfigNames
		_ = c.RegisterFlagCompletionFunc("context", completeContextsForArgIdx(0))
	}
	cmd.AddCommand(enableCmd, disableCmd, showCmd)
	return cmd
}

// printAlerts renders a single Alerts block with a label. Fills BlockedVerbs
// with the default set when the entry doesn't specify its own, so users see
// the *effective* policy rather than a confusing empty list.
func printAlerts(out io.Writer, label string, a state.Alerts) {
	_, _ = fmt.Fprintf(out, "%s:\n", label)
	_, _ = fmt.Fprintf(out, "  Enabled:               %t\n", a.Enabled)
	_, _ = fmt.Fprintf(out, "  Require confirmation:  %t\n", a.RequireConfirmation)
	_, _ = fmt.Fprintf(out, "  Confirm cluster name:  %t\n", a.ConfirmClusterName)
	verbs := a.BlockedVerbs
	if len(verbs) == 0 {
		verbs = state.DefaultBlockedVerbs()
	}
	_, _ = fmt.Fprintf(out, "  Blocked verbs:         %s\n", strings.Join(verbs, ", "))
}

func sortedAlertKeys(m map[string]state.Alerts) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
