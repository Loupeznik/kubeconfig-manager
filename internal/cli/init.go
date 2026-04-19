package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/loupeznik/kubeconfig-manager/internal/shell"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// newInitCmd wires `kcm init` — a one-shot interactive walkthrough for new
// users. Populates the palette, offers to install the shell hook, and prints
// a starter starship module snippet. Everything it does is reversible: the
// palette can be edited later, the shell hook uninstalls cleanly, and the
// starship block is copy-paste, not written automatically.
func newInitCmd() *cobra.Command {
	var opts initOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "First-run walkthrough: palette seed, shell hook, starship snippet",
		Long: "Interactive setup that asks a handful of questions and performs the common " +
			"first-time steps: adding a starter tag palette, installing the shell hook " +
			"(with optional kubectl/helm aliases), and printing a starship custom-module " +
			"snippet. Pass --yes to skip prompts and accept every default (useful for " +
			"automation and for testing).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.AssumeYes, "yes", false, "Skip interactive prompts and accept defaults")
	cmd.Flags().BoolVar(&opts.SkipPalette, "skip-palette", false, "Do not seed the tag palette")
	cmd.Flags().BoolVar(&opts.SkipShellHook, "skip-shell-hook", false, "Do not install the shell hook")
	cmd.Flags().StringVar(&opts.RC, "rc", "", "Override rc file path for the shell hook install (default: detected per shell)")
	return cmd
}

// initOptions bundles the flags plumbed into runInit. Extracted so tests can
// construct one directly and exercise the wizard without going through cobra.
type initOptions struct {
	AssumeYes     bool
	SkipPalette   bool
	SkipShellHook bool
	RC            string
}

func runInit(ctx context.Context, out io.Writer, opts initOptions) error {
	_, _ = fmt.Fprintln(out, "kubeconfig-manager — first-run setup")
	_, _ = fmt.Fprintln(out)

	// --- Step 1: seed palette -----------------------------------------------
	seedPalette := !opts.SkipPalette
	paletteTags := defaultStarterPalette()
	if seedPalette && !opts.AssumeYes {
		if err := huh.NewConfirm().
			Title("Seed the tag palette with a starter set?").
			Description("Tags: " + strings.Join(paletteTags, ", ")).
			Affirmative("Yes").
			Negative("Skip").
			Value(&seedPalette).
			Run(); err != nil {
			return err
		}
	}
	if seedPalette {
		store, err := state.DefaultStore()
		if err != nil {
			return err
		}
		var added []string
		if err := store.Mutate(ctx, func(cfg *state.Config) error {
			cfg.EnsurePaletteFromEntries()
			added = cfg.AddAvailableTags(paletteTags...)
			return nil
		}); err != nil {
			return fmt.Errorf("seed palette: %w", err)
		}
		if len(added) == 0 {
			_, _ = fmt.Fprintln(out, "[ok] palette already contained these tags; nothing to add")
		} else {
			_, _ = fmt.Fprintf(out, "[ok] added to palette: %s\n", strings.Join(added, ", "))
		}
	} else {
		_, _ = fmt.Fprintln(out, "[skip] palette")
	}

	// --- Step 2: shell hook -------------------------------------------------
	installHook := !opts.SkipShellHook
	aliasKubectl := true
	aliasHelm := false
	detectedShell := shell.Detect()
	if installHook && !opts.AssumeYes {
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Install the %s shell hook?", detectedShell)).
			Description("Adds a fenced block to your rc file with the kcm() function and optional kubectl/helm aliases.").
			Affirmative("Yes").
			Negative("Skip").
			Value(&installHook).
			Run(); err != nil {
			return err
		}
		if installHook {
			if err := huh.NewConfirm().
				Title("Also alias kubectl to route through the destructive-action guard?").
				Affirmative("Yes (recommended)").
				Negative("No").
				Value(&aliasKubectl).
				Run(); err != nil {
				return err
			}
			if err := huh.NewConfirm().
				Title("Also alias helm to route through the helm values-path guard?").
				Affirmative("Yes").
				Negative("No").
				Value(&aliasHelm).
				Run(); err != nil {
				return err
			}
		}
	}
	if installHook {
		rc := opts.RC
		if rc == "" {
			var err error
			rc, err = shell.RCPath(detectedShell)
			if err != nil {
				return fmt.Errorf("rc path: %w", err)
			}
		}
		hook, err := shell.RenderHook(detectedShell, shell.HookOptions{
			AliasKubectl: aliasKubectl,
			AliasHelm:    aliasHelm,
		})
		if err != nil {
			return err
		}
		res, err := shell.InstallHook(rc, hook)
		if err != nil {
			return fmt.Errorf("install hook: %w", err)
		}
		switch {
		case res.Created:
			_, _ = fmt.Fprintf(out, "[ok] created %s and installed %s hook\n", res.RCPath, detectedShell)
		case res.Updated:
			_, _ = fmt.Fprintf(out, "[ok] updated %s hook in %s\n", detectedShell, res.RCPath)
		default:
			_, _ = fmt.Fprintf(out, "[ok] %s hook already current in %s\n", detectedShell, res.RCPath)
		}
		_, _ = fmt.Fprintf(out, "      → restart your shell or run: source %s\n", res.RCPath)
	} else {
		_, _ = fmt.Fprintln(out, "[skip] shell hook")
	}

	// --- Step 3: starship snippet (print-only) ------------------------------
	printStarship := true
	if !opts.AssumeYes {
		if err := huh.NewConfirm().
			Title("Print a starship custom-module snippet?").
			Description("Shows a config block you can paste into ~/.config/starship.toml.").
			Affirmative("Yes").
			Negative("Skip").
			Value(&printStarship).
			Run(); err != nil {
			return err
		}
	}
	if printStarship {
		printStarshipSnippet(out)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Next steps:")
	_, _ = fmt.Fprintln(out, "  - run 'kcm list' to see your kubeconfigs")
	_, _ = fmt.Fprintln(out, "  - run 'kcm tui' for the interactive picker")
	_, _ = fmt.Fprintln(out, "  - run 'kcm doctor' anytime to recheck your setup")
	return nil
}

func defaultStarterPalette() []string {
	return []string{"prod", "staging", "dev", "sandbox", "eu", "us", "critical"}
}

func printStarshipSnippet(out io.Writer) {
	binary := filepath.Base(starshipBinaryName())
	_, _ = fmt.Fprintln(out, "\nStarship snippet — paste into ~/.config/starship.toml:")
	_, _ = fmt.Fprintln(out, "  [custom.kcm]")
	_, _ = fmt.Fprintf(out, "  command = \"%s starship\"\n", binary)
	_, _ = fmt.Fprintf(out, "  when = \"%s starship | grep -q .\"\n", binary)
	_, _ = fmt.Fprintln(out, "  format = \"[$output]($style) \"")
	_, _ = fmt.Fprintln(out, "  style = \"bold yellow\"")
}

// starshipBinaryName returns the binary path used in the starship snippet.
// Prefers the user's invocation name (os.Args[0]) when it resolves to
// something friendlier than an absolute temp path (testscript-style); falls
// back to the canonical binary name.
func starshipBinaryName() string {
	if len(os.Args) > 0 {
		base := filepath.Base(os.Args[0])
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return "kubeconfig-manager"
}
