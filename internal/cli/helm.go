package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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
				if err := guard.ConfirmHelm(decision); err != nil {
					if errors.Is(err, guard.ErrDeclined) {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "aborted")
						os.Exit(1)
					}
					return err
				}
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
		newHelmGuardSetPatternCmd(),
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

func newHelmGuardSetPatternCmd() *cobra.Command {
	var dir, file string
	cmd := &cobra.Command{
		Use:   "set-pattern <pattern>",
		Short: "Set the path pattern used to derive cluster/env names (e.g. \"clusters/{name}/\")",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := args[0]
			if !strings.Contains(pattern, "{name}") {
				return fmt.Errorf("pattern %q must contain the {name} placeholder", pattern)
			}
			return mutateHelmGuard(cmd, dir, file, func(hg *state.HelmGuard) {
				hg.Pattern = pattern
			}, "pattern set to "+pattern)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&file, "file", "", "Apply only to this kubeconfig (default: global)")
	_ = cmd.RegisterFlagCompletionFunc("file", completionFuncForFileFlag)
	return cmd
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
	_, _ = fmt.Fprintf(out, "  Enabled:  %t\n", hg.Enabled)
	pattern := hg.Pattern
	if pattern == "" {
		pattern = state.DefaultHelmPattern + " (default)"
	}
	_, _ = fmt.Fprintf(out, "  Pattern:  %s\n", pattern)
	tokens := hg.EnvTokens
	if len(tokens) == 0 {
		tokens = state.DefaultEnvTokens()
		_, _ = fmt.Fprintf(out, "  Tokens:   %s (default)\n", strings.Join(tokens, ", "))
	} else {
		_, _ = fmt.Fprintf(out, "  Tokens:   %s\n", strings.Join(tokens, ", "))
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "helm-guard %s (%s)\n", status, scope)
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
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "helm-guard %s for %s\n", status, path)
		return nil
	})
}

// completionFuncForFileFlag completes the --file flag for helm-guard commands
// by listing kubeconfig names in --dir.
func completionFuncForFileFlag(cmd *cobra.Command, args []string, tc string) ([]string, cobra.ShellCompDirective) {
	return completeKubeconfigNames(cmd, args, tc)
}
