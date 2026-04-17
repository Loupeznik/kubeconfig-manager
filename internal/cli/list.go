package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

func newListCmd() *cobra.Command {
	var dir string
	var verbose bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List kubeconfig files in the managed directory",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedDir, err := resolveDir(dir)
			if err != nil {
				return err
			}
			result, err := kubeconfig.ScanDir(resolvedDir)
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

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "FILE\tCONTEXTS\tCURRENT\tTAGS\tALERTS")
			for _, f := range result.Files {
				current := f.Config.CurrentContext
				if current == "" {
					current = "-"
				}
				hash, err := kubeconfig.HashFile(f.Path)
				if err != nil {
					return err
				}
				entry := cfg.Entries[hash]
				tags := "-"
				if len(entry.Tags) > 0 {
					tags = strings.Join(entry.Tags, ",")
				}
				alerts := "off"
				if entry.Alerts.Enabled {
					alerts = "on"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", f.Name(), len(f.Config.Contexts), current, tags, alerts)
			}
			if err := tw.Flush(); err != nil {
				return err
			}

			if verbose {
				for _, w := range result.Warnings {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: %v\n", w.Path, w.Err)
				}
			}
			if len(result.Files) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "no kubeconfig files found in %s\n", resolvedDir)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory (default: ~/.kube)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show files that were skipped because they couldn't be parsed as kubeconfigs")
	return cmd
}

func newShowCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "show <name-or-file>",
		Short: "Show contexts, clusters, and users of a kubeconfig file",
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
			f, err := kubeconfig.Load(path)
			if err != nil {
				return err
			}
			return printFileDetail(cmd, f)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Kubeconfig directory for bare-name lookups (default: ~/.kube)")
	return cmd
}

func newContextsCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "contexts",
		Short: "List contexts in the default kubeconfig (~/.kube/config)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if path == "" {
				def, err := kubeconfig.DefaultPath()
				if err != nil {
					return err
				}
				path = def
			}
			f, err := kubeconfig.Load(path)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "CONTEXT\tCLUSTER\tUSER\tNAMESPACE\tCURRENT")
			for _, name := range f.ContextNames() {
				ctx := f.Config.Contexts[name]
				marker := ""
				if name == f.Config.CurrentContext {
					marker = "*"
				}
				ns := ctx.Namespace
				if ns == "" {
					ns = "-"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", name, ctx.Cluster, ctx.AuthInfo, ns, marker)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "Kubeconfig file to inspect (default: ~/.kube/config)")
	return cmd
}

func printFileDetail(cmd *cobra.Command, f *kubeconfig.File) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "File:             %s\n", f.Path)
	_, _ = fmt.Fprintf(out, "Current context:  %s\n", valueOrDash(f.Config.CurrentContext))
	_, _ = fmt.Fprintf(out, "API version:      %s\n", valueOrDash(f.Config.APIVersion))
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Contexts:")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NAME\tCLUSTER\tUSER\tNAMESPACE")
	for _, name := range f.ContextNames() {
		ctx := f.Config.Contexts[name]
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", name, ctx.Cluster, ctx.AuthInfo, valueOrDash(ctx.Namespace))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Clusters:")
	for _, name := range f.ClusterNames() {
		_, _ = fmt.Fprintf(out, "  - %s (%s)\n", name, f.Config.Clusters[name].Server)
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Users:")
	for _, name := range f.UserNames() {
		_, _ = fmt.Fprintf(out, "  - %s\n", name)
	}

	return nil
}

func resolveDir(dir string) (string, error) {
	if dir != "" {
		if strings.HasPrefix(dir, "~/") {
			home, err := homeDir()
			if err != nil {
				return "", err
			}
			return strings.Replace(dir, "~", home, 1), nil
		}
		return dir, nil
	}
	return kubeconfig.DefaultDir()
}

func valueOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
