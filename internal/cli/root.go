package cli

import (
	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kcm",
		Short: "Manage kubeconfig files and kubectl contexts",
		Long: "kubeconfig-manager (kcm) is a TUI + CLI for managing local kubeconfig files " +
			"and kubectl contexts, with tagging and destructive-action guardrails.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newListCmd(),
		newShowCmd(),
		newContextsCmd(),
		newUseCmd(),
		newTagCmd(),
		newAlertCmd(),
		newImportCmd(),
		newSplitCmd(),
		newMergeCmd(),
		newKubectlCmd(),
		newTUICmd(),
		newInstallShellHookCmd(),
	)

	return root
}
