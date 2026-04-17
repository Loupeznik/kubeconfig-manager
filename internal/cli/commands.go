package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/guard"
	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/shell"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
	"github.com/loupeznik/kubeconfig-manager/internal/tui"
)

var errNotImplemented = errors.New("not implemented yet")

func newUseCmd() *cobra.Command {
	var dir string
	var shellFlag string
	cmd := &cobra.Command{
		Use:   "use <name-or-file>",
		Short: "Print a shell snippet that exports KUBECONFIG to the selected file",
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
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			sh, err := shell.Resolve(shellFlag)
			if err != nil {
				return err
			}
			line, err := shell.ExportLine(sh, abs)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&shellFlag, "shell", "", "Shell to emit for: bash, zsh, pwsh (auto-detected if unset)")
	return cmd
}

func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "Import a kubeconfig file into the default ~/.kube/config",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
}

func newSplitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "split <context> <out-file>",
		Short: "Split a context out of ~/.kube/config into its own file",
		Args:  cobra.ExactArgs(2),
		RunE:  func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
}

func newMergeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "merge <a> <b> <out>",
		Short: "Merge two kubeconfig files into a new file",
		Args:  cobra.ExactArgs(3),
		RunE:  func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
}

func newKubectlCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "kubectl [args...]",
		Short:              "Run kubectl through the destructive-action guard",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			decision, err := guard.Evaluate(cmd.Context(), store, os.Getenv("KUBECONFIG"), args)
			if err != nil {
				return err
			}
			if decision.Alert() {
				if err := guard.Confirm(decision); err != nil {
					if errors.Is(err, guard.ErrDeclined) {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "aborted")
						os.Exit(1)
					}
					return err
				}
			}
			return guard.Exec(args)
		},
	}
	return cmd
}

func newTUICmd() *cobra.Command {
	var dir string
	var shellFlag string
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			store, err := state.DefaultStore()
			if err != nil {
				return err
			}
			selected, err := tui.Run(cmd.Context(), resolvedDir, store)
			if err != nil {
				return err
			}
			if selected != "" {
				abs, err := filepath.Abs(selected)
				if err != nil {
					return err
				}
				sh, err := shell.Resolve(shellFlag)
				if err != nil {
					return err
				}
				line, err := shell.ExportLine(sh, abs)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().StringVar(&shellFlag, "shell", "", "Shell to emit for: bash, zsh, pwsh (auto-detected if unset)")
	return cmd
}

func newInstallShellHookCmd() *cobra.Command {
	var shellFlag string
	var rcFlag string
	var aliasKubectl bool

	cmd := &cobra.Command{
		Use:   "install-shell-hook",
		Short: "Install shell integration (kcm function, optional kubectl alias)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sh, err := shell.Resolve(shellFlag)
			if err != nil {
				return err
			}
			rcPath := rcFlag
			if rcPath == "" {
				rcPath, err = shell.RCPath(sh)
				if err != nil {
					return err
				}
			}
			hook, err := shell.RenderHook(sh, shell.HookOptions{AliasKubectl: aliasKubectl})
			if err != nil {
				return err
			}
			res, err := shell.InstallHook(rcPath, hook)
			if err != nil {
				return err
			}
			switch {
			case res.Created:
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "created %s and installed %s hook\n", res.RCPath, sh)
			case res.Updated:
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "updated %s hook in %s\n", sh, res.RCPath)
			default:
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "installed %s hook in %s\n", sh, res.RCPath)
			}
			if aliasKubectl {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "note: kubectl alias installed — all kubectl invocations route through kubeconfig-manager")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "restart your shell or source the rc file to activate")
			return nil
		},
	}
	cmd.Flags().StringVar(&shellFlag, "shell", "", "bash, zsh, or pwsh (auto-detected if unset)")
	cmd.Flags().StringVar(&rcFlag, "rc", "", "rc file path (default depends on shell)")
	cmd.Flags().BoolVar(&aliasKubectl, "alias-kubectl", false, "Also alias kubectl to route through the guard (opt-in)")
	return cmd
}

func newUninstallShellHookCmd() *cobra.Command {
	var shellFlag string
	var rcFlag string

	cmd := &cobra.Command{
		Use:   "uninstall-shell-hook",
		Short: "Remove the shell integration block from the rc file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sh, err := shell.Resolve(shellFlag)
			if err != nil {
				return err
			}
			rcPath := rcFlag
			if rcPath == "" {
				rcPath, err = shell.RCPath(sh)
				if err != nil {
					return err
				}
			}
			removed, err := shell.UninstallHook(rcPath)
			if err != nil {
				return err
			}
			if removed {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed kubeconfig-manager hook from %s\n", rcPath)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no kubeconfig-manager hook found in %s\n", rcPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&shellFlag, "shell", "", "bash, zsh, or pwsh (auto-detected if unset)")
	cmd.Flags().StringVar(&rcFlag, "rc", "", "rc file path (default depends on shell)")
	return cmd
}
