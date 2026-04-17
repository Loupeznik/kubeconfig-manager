package guard

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

var ErrNoTTY = errors.New("no TTY available for confirmation prompt")

func Confirm(d Decision) error {
	if !d.Alert() {
		return nil
	}
	if !hasTTY() {
		return fmt.Errorf("%w (alerts enabled; run in an interactive shell or disable alerts)", ErrNoTTY)
	}

	_, _ = fmt.Fprintln(os.Stderr)
	_, _ = fmt.Fprintln(os.Stderr, d.Describe())

	if d.RequireClusterName() {
		names := d.ExpectedClusterNames()
		if len(names) == 0 {
			return errors.New("cluster-name confirmation required but no cluster resolved from kubeconfig")
		}
		return confirmClusterName(names)
	}

	var approved bool
	err := huh.NewConfirm().
		Title(fmt.Sprintf("Proceed with kubectl %s?", d.Verb)).
		Affirmative("Yes, proceed").
		Negative("No, abort").
		Value(&approved).
		Run()
	if err != nil {
		return err
	}
	if !approved {
		return ErrDeclined
	}
	return nil
}

func confirmClusterName(expected []string) error {
	expectedSet := map[string]bool{}
	for _, n := range expected {
		expectedSet[n] = true
	}

	var typed string
	prompt := fmt.Sprintf("Type the active cluster name to confirm (%s):", strings.Join(expected, " | "))
	err := huh.NewInput().
		Title(prompt).
		Value(&typed).
		Validate(func(s string) error {
			if !expectedSet[strings.TrimSpace(s)] {
				return fmt.Errorf("name does not match any expected cluster")
			}
			return nil
		}).
		Run()
	if err != nil {
		return err
	}
	if !expectedSet[strings.TrimSpace(typed)] {
		return ErrDeclined
	}
	return nil
}

func hasTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}
