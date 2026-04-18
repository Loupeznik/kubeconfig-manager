package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/audit"
)

// newAuditCmd wires `kcm audit` — a tail view over the guard-prompt log
// (kubectl and helm). Intentionally minimal: the file format is plain text
// key=value, so users can reach for grep/awk/less/sed when they want more.
func newAuditCmd() *cobra.Command {
	var tailN int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show the guard-prompt audit log (kubectl + helm approvals / aborts)",
		Long: "Prints the last --tail entries from the audit log written by the kubectl " +
			"and helm guards on every prompt. The log lives under XDG_DATA_HOME/kubeconfig-manager/audit.log " +
			"and uses a one-line key=value format so grep/awk work naturally.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := audit.DefaultPath()
			if err != nil {
				return err
			}
			lines, err := audit.Tail(path, tailN)
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no audit entries yet (%s)\n", path)
				return nil
			}
			for _, l := range lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), l)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&tailN, "tail", 20, "Number of most-recent entries to show (0 = all)")
	return cmd
}
