package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/audit"
	"github.com/loupeznik/kubeconfig-manager/internal/guard"
	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

func newHelmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "helm [args...]",
		Short:              "Run helm through the values-path / context mismatch guard",
		Long:               "Invokes helm with the given arguments. When the active kubeconfig has the helm-guard enabled, kcm parses -f/--values flags, derives a cluster/env name from each values-file path, and compares it to the active kubectl context. On a significant mismatch (e.g. path says k8s-test-01, context is k8s-prod-01), kcm prompts for confirmation before exec'ing helm.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			decision, err := guard.EvaluateHelm(cmd.Context(), store, os.Getenv("KUBECONFIG"), args)
			if err != nil {
				return err
			}
			if decision.Alert() {
				ev := audit.Event{
					Tool:      "helm",
					Verb:      firstHelmVerb(args),
					Context:   decision.ContextName,
					Cluster:   decision.ClusterName,
					Decision:  "approved",
					Severity:  decision.Triggers[0].Severity.String(),
					ValuePath: decision.Triggers[0].ValuesPath,
				}
				if err := guard.ConfirmHelm(decision); err != nil {
					switch {
					case errors.Is(err, guard.ErrDeclined):
						ev.Decision = "declined"
						_ = audit.Append(ev)
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "aborted")
						os.Exit(1)
					case errors.Is(err, guard.ErrNoTTY):
						ev.Decision = "no-tty"
						_ = audit.Append(ev)
						return err
					default:
						return err
					}
				}
				_ = audit.Append(ev)
			}
			code, err := guard.ExecHelm(args, guard.ExecOptions{})
			if err != nil {
				return err
			}
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
	return cmd
}

// firstHelmVerb returns the first non-flag token from args, which is helm's
// subcommand ("upgrade", "install", ...). Empty string when args start with
// a flag or are empty.
func firstHelmVerb(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

// ============================================================================
// helm-guard config management
// ============================================================================

func newHelmGuardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "helm-guard",
		Short:   "Configure the helm values-path / context mismatch guard",
		Aliases: []string{"helmguard"},
	}
	cmd.AddCommand(
		newHelmGuardEnableCmd(),
		newHelmGuardDisableCmd(),
		newHelmGuardShowCmd(),
		newHelmGuardSetPatternsCmd(),
		newHelmGuardAddPatternCmd(),
		newHelmGuardRemovePatternCmd(),
		newHelmGuardFallbackCmd(),
	)
	return cmd
}

func newHelmGuardEnableCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable the helm guard globally (default) or for a specific kubeconfig with --file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mutateHelmGuard(cmd, dir, file, func(hg *state.HelmGuard) {
				hg.Enabled = true
			}, "enabled")
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Apply only to this kubeconfig (default: global)")
	cmd.ValidArgsFunction = func(c *cobra.Command, args []string, tc string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	return cmd
}

func newHelmGuardDisableCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable the helm guard globally (default) or override per --file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mutateHelmGuard(cmd, dir, file, func(hg *state.HelmGuard) {
				hg.Enabled = false
			}, "disabled")
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Apply only to this kubeconfig (default: global)")
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	return cmd
}

func newHelmGuardSetPatternsCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:     "set-patterns <pattern...>",
		Aliases: []string{"set-pattern"},
		Short:   "Replace the path-pattern list used to derive cluster/env names",
		Long: "Replaces the entire pattern list with the given patterns. Each pattern " +
			"must contain the {name} placeholder (capture stops at the next slash). " +
			"Patterns are tried in order and the first match wins. See also add-pattern, " +
			"remove-pattern, fallback.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validatePatterns(args); err != nil {
				return err
			}
			patterns := append([]string(nil), args...)
			return mutateHelmGuard(cmd, dir, file, func(hg *state.HelmGuard) {
				hg.Patterns = patterns
			}, "patterns set to "+strings.Join(patterns, ", "))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Apply only to this kubeconfig (default: global)")
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	return cmd
}

func newHelmGuardAddPatternCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:   "add-pattern <pattern...>",
		Short: "Append one or more patterns to the existing list",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validatePatterns(args); err != nil {
				return err
			}
			var added []string
			if err := mutateHelmGuard(cmd, dir, file, func(hg *state.HelmGuard) {
				existing := map[string]bool{}
				for _, p := range hg.Patterns {
					existing[p] = true
				}
				for _, p := range args {
					if existing[p] {
						continue
					}
					existing[p] = true
					hg.Patterns = append(hg.Patterns, p)
					added = append(added, p)
				}
			}, ""); err != nil {
				return err
			}
			if len(added) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no new patterns added (already present)")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "added patterns: %s\n", strings.Join(added, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Apply only to this kubeconfig (default: global)")
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	return cmd
}

func newHelmGuardRemovePatternCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:   "remove-pattern <pattern...>",
		Short: "Drop one or more patterns from the existing list",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			drop := map[string]bool{}
			for _, p := range args {
				drop[p] = true
			}
			var removed []string
			if err := mutateHelmGuard(cmd, dir, file, func(hg *state.HelmGuard) {
				kept := hg.Patterns[:0]
				for _, p := range hg.Patterns {
					if drop[p] {
						removed = append(removed, p)
						continue
					}
					kept = append(kept, p)
				}
				hg.Patterns = kept
			}, ""); err != nil {
				return err
			}
			if len(removed) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no matching patterns to remove")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed patterns: %s\n", strings.Join(removed, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Apply only to this kubeconfig (default: global)")
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	return cmd
}

func newHelmGuardFallbackCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:   "fallback <on|off>",
		Short: "Toggle the pattern-less global fallback (compare path tokens directly when no pattern matches)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			val, err := parseOnOff(args[0])
			if err != nil {
				return err
			}
			label := "off"
			if val {
				label = "on"
			}
			return mutateHelmGuard(cmd, dir, file, func(hg *state.HelmGuard) {
				hg.GlobalFallback = val
			}, "global fallback "+label)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Apply only to this kubeconfig (default: global)")
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	cmd.ValidArgs = []string{"on", "off"}
	return cmd
}

// validatePatterns rejects patterns missing the {name} placeholder. Shared
// by set-patterns and add-pattern so the error message stays consistent.
func validatePatterns(patterns []string) error {
	for _, p := range patterns {
		if !strings.Contains(p, "{name}") {
			return fmt.Errorf("pattern %q must contain the {name} placeholder", p)
		}
	}
	return nil
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "true", "yes", "enable", "enabled", "1":
		return true, nil
	case "off", "false", "no", "disable", "disabled", "0":
		return false, nil
	}
	return false, fmt.Errorf("expected on|off, got %q", s)
}

func newHelmGuardShowCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the effective helm guard policy (global, or --file-specific)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			cfg, err := store.Load(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if file == "" {
				printHelmGuard(out, "Global", cfg.HelmGuard)
				return nil
			}
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			path, err := kubeconfig.ResolvePath(file, resolvedDir)
			if err != nil {
				return err
			}
			id, err := kubeconfig.IdentifyFile(path)
			if err != nil {
				return err
			}
			entry, _ := cfg.GetEntry(id.StableHash, id.ContentHash)
			_, _ = fmt.Fprintf(out, "File: %s\n", path)
			if entry.HelmGuard != nil {
				printHelmGuard(out, "Per-entry override", *entry.HelmGuard)
			} else {
				_, _ = fmt.Fprintln(out, "(no per-entry override; inherits global)")
			}
			_, _ = fmt.Fprintln(out)
			printHelmGuard(out, "Effective (resolved)", entry.ResolveHelmGuard(cfg.HelmGuard))
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Show policy for this kubeconfig (default: global only)")
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	return cmd
}

func printHelmGuard(out io.Writer, label string, hg state.HelmGuard) {
	_, _ = fmt.Fprintf(out, "%s:\n", label)
	_, _ = fmt.Fprintf(out, "  Enabled:          %t\n", hg.Enabled)
	patterns := hg.Patterns
	patternSuffix := ""
	if len(patterns) == 0 {
		patterns = []string{state.DefaultHelmPattern}
		patternSuffix = " (default)"
	}
	_, _ = fmt.Fprintf(out, "  Patterns:         %s%s\n", strings.Join(patterns, ", "), patternSuffix)
	_, _ = fmt.Fprintf(out, "  Global fallback:  %t\n", hg.GlobalFallback)
	tokens := hg.EnvTokens
	if len(tokens) == 0 {
		tokens = state.DefaultEnvTokens()
		_, _ = fmt.Fprintf(out, "  Tokens:           %s (default)\n", strings.Join(tokens, ", "))
	} else {
		_, _ = fmt.Fprintf(out, "  Tokens:           %s\n", strings.Join(tokens, ", "))
	}
}

// mutateHelmGuard applies `mutate` to either cfg.HelmGuard (when file is empty)
// or the per-entry HelmGuard of the resolved file. Creates the per-entry
// struct if it doesn't exist.
func mutateHelmGuard(cmd *cobra.Command, dir, file string, mutate func(*state.HelmGuard), status string) error {
	store, err := state.DefaultStore()
	if err != nil {
		return err
	}
	scope := "global"
	return store.Mutate(cmd.Context(), func(cfg *state.Config) error {
		if file == "" {
			mutate(&cfg.HelmGuard)
			if status != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "helm-guard %s (%s)\n", status, scope)
			}
			return nil
		}
		resolvedDir, err := resolveDir(dir)
		if err != nil {
			return err
		}
		path, err := kubeconfig.ResolvePath(file, resolvedDir)
		if err != nil {
			return err
		}
		id, err := kubeconfig.IdentifyFile(path)
		if err != nil {
			return err
		}
		entry := cfg.TakeEntry(id.StableHash, id.ContentHash)
		if entry.HelmGuard == nil {
			entry.HelmGuard = &state.HelmGuard{}
		}
		mutate(entry.HelmGuard)
		entry.PathHint = filepath.Base(path)
		entry.Touch()
		cfg.Entries[id.StableHash] = entry
		if status != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "helm-guard %s for %s\n", status, path)
		}
		return nil
	})
}

// completionFuncForFileFlag completes the --file flag for helm-guard commands
// by listing kubeconfig names in --dir.
func completionFuncForFileFlag(cmd *cobra.Command, args []string, tc string) ([]string, cobra.ShellCompDirective) {
	return completeKubeconfigNames(cmd, args, tc)
}
