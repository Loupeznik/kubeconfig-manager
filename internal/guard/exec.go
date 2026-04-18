package guard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ExecOptions controls the process plumbing of Exec. Zero values use the
// parent's stdin/stdout/stderr — which is what callers like `kcm kubectl`
// want. Tests can inject buffers.
type ExecOptions struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string // nil → inherit os.Environ()
}

// Exec runs kubectl with args and returns the child's exit code. Any error
// returned is a failure to launch or propagate the exit code (e.g. kubectl
// missing from PATH); a non-zero exit code from kubectl itself comes back as
// (code, nil).
//
// Callers that need the child's code to terminate the whole process should
// wrap this with os.Exit(code).
func Exec(args []string, opts ExecOptions) (int, error) {
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		return 0, fmt.Errorf("kubectl not found on PATH: %w", err)
	}

	cmd := exec.Command(kubectl, args...)
	cmd.Stdin = opts.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = opts.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = opts.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	cmd.Env = opts.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}

	err = cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, fmt.Errorf("kubectl exec: %w", err)
}
