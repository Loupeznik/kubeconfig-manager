package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

var errNotImplemented = errors.New("not implemented yet")

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List kubeconfig files in the managed directory",
		RunE:  func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
}

func newUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <name-or-file>",
		Short: "Print a shell snippet that exports KUBECONFIG to the selected file",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
	cmd.Flags().String("shell", "", "Shell to emit for: bash, zsh, pwsh (auto-detected if unset)")
	return cmd
}

func newTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage tags on kubeconfig files",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "add <file> <tag...>", Short: "Add tags", RunE: func(cmd *cobra.Command, args []string) error { return errNotImplemented }},
		&cobra.Command{Use: "remove <file> <tag...>", Short: "Remove tags", RunE: func(cmd *cobra.Command, args []string) error { return errNotImplemented }},
		&cobra.Command{Use: "list [file]", Short: "List tags", RunE: func(cmd *cobra.Command, args []string) error { return errNotImplemented }},
	)
	return cmd
}

func newAlertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "Configure destructive-action alerts per kubeconfig",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "enable <file>", Short: "Enable alerts", RunE: func(cmd *cobra.Command, args []string) error { return errNotImplemented }},
		&cobra.Command{Use: "disable <file>", Short: "Disable alerts", RunE: func(cmd *cobra.Command, args []string) error { return errNotImplemented }},
		&cobra.Command{Use: "show <file>", Short: "Show alert policy", RunE: func(cmd *cobra.Command, args []string) error { return errNotImplemented }},
	)
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
		RunE:               func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
	return cmd
}

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		RunE:  func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
}

func newInstallShellHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install-shell-hook",
		Short: "Install shell integration (kcm function, optional kubectl alias)",
		RunE:  func(cmd *cobra.Command, args []string) error { return errNotImplemented },
	}
	cmd.Flags().String("shell", "", "bash, zsh, or pwsh (auto-detected if unset)")
	cmd.Flags().Bool("alias-kubectl", false, "Also alias kubectl to route through the guard (opt-in)")
	return cmd
}
