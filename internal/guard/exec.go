package guard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func Exec(args []string) error {
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found on PATH: %w", err)
	}

	cmd := exec.Command(kubectl, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	err = cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return fmt.Errorf("kubectl exec: %w", err)
}
