package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
)

func newImportCmd() *cobra.Command {
	var into string
	var onConflict string

	cmd := &cobra.Command{
		Use:   "import <source>",
		Short: "Merge a kubeconfig file into the default ~/.kube/config (or --into)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcPath := args[0]
			srcCfg, err := clientcmd.LoadFromFile(srcPath)
			if err != nil {
				return fmt.Errorf("load source: %w", err)
			}

			destPath := into
			if destPath == "" {
				destPath, err = kubeconfig.DefaultPath()
				if err != nil {
					return err
				}
			}

			destCfg, err := loadOrEmpty(destPath)
			if err != nil {
				return err
			}

			policy, err := kubeconfig.ParseConflictPolicy(onConflict)
			if err != nil {
				return err
			}

			merged, _, err := kubeconfig.Merge(destCfg, srcCfg, policy)
			if err != nil {
				var col kubeconfig.Collisions
				if errors.As(err, &col) {
					return err
				}
				return err
			}

			if err := clientcmd.WriteToFile(*merged, destPath); err != nil {
				return fmt.Errorf("write %s: %w", destPath, err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "imported %s into %s\n", srcPath, destPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&into, "into", "", "Destination kubeconfig (default: ~/.kube/config)")
	cmd.Flags().StringVar(&onConflict, "on-conflict", "error", "error | skip | overwrite")
	return cmd
}

func newSplitCmd() *cobra.Command {
	var from string
	var remove bool

	cmd := &cobra.Command{
		Use:   "split <context> <out-file>",
		Short: "Extract a context (with its cluster and user) into its own file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			contextName := args[0]
			outPath := args[1]

			srcPath := from
			if srcPath == "" {
				var err error
				srcPath, err = kubeconfig.DefaultPath()
				if err != nil {
					return err
				}
			}

			srcCfg, err := clientcmd.LoadFromFile(srcPath)
			if err != nil {
				return fmt.Errorf("load source: %w", err)
			}

			extracted, err := kubeconfig.Extract(srcCfg, contextName)
			if err != nil {
				return err
			}

			if _, err := os.Stat(outPath); err == nil {
				return fmt.Errorf("destination %s already exists (refusing to overwrite)", outPath)
			}

			if err := clientcmd.WriteToFile(*extracted, outPath); err != nil {
				return fmt.Errorf("write %s: %w", outPath, err)
			}

			if remove {
				pruned, err := kubeconfig.Remove(srcCfg, contextName)
				if err != nil {
					return err
				}
				if err := clientcmd.WriteToFile(*pruned, srcPath); err != nil {
					return fmt.Errorf("update source after removal: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "extracted %s to %s and removed from %s\n", contextName, outPath, srcPath)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "extracted %s to %s\n", contextName, outPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Source kubeconfig (default: ~/.kube/config)")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove the extracted context from the source file")
	return cmd
}

func newMergeCmd() *cobra.Command {
	var onConflict string
	var force bool

	cmd := &cobra.Command{
		Use:   "merge <a> <b> <out>",
		Short: "Merge two kubeconfig files into a new file",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, b, out := args[0], args[1], args[2]

			cfgA, err := clientcmd.LoadFromFile(a)
			if err != nil {
				return fmt.Errorf("load %s: %w", a, err)
			}
			cfgB, err := clientcmd.LoadFromFile(b)
			if err != nil {
				return fmt.Errorf("load %s: %w", b, err)
			}

			policy, err := kubeconfig.ParseConflictPolicy(onConflict)
			if err != nil {
				return err
			}

			merged, _, err := kubeconfig.Merge(cfgA, cfgB, policy)
			if err != nil {
				return err
			}

			if _, err := os.Stat(out); err == nil && !force {
				return fmt.Errorf("destination %s already exists (pass --force to overwrite)", out)
			}

			if err := clientcmd.WriteToFile(*merged, out); err != nil {
				return fmt.Errorf("write %s: %w", out, err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "merged %s + %s -> %s\n", a, b, out)
			return nil
		},
	}
	cmd.Flags().StringVar(&onConflict, "on-conflict", "error", "error | skip | overwrite")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite the destination if it exists")
	return cmd
}

func loadOrEmpty(path string) (*clientcmdapi.Config, error) {
	cfg, err := clientcmd.LoadFromFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &clientcmdapi.Config{
			APIVersion: "v1",
			Kind:       "Config",
			Clusters:   map[string]*clientcmdapi.Cluster{},
			AuthInfos:  map[string]*clientcmdapi.AuthInfo{},
			Contexts:   map[string]*clientcmdapi.Context{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return cfg, nil
}
