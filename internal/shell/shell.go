package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Shell int

const (
	Unknown Shell = iota
	Bash
	Zsh
	PowerShell
)

func (s Shell) String() string {
	switch s {
	case Bash:
		return "bash"
	case Zsh:
		return "zsh"
	case PowerShell:
		return "pwsh"
	}
	return "unknown"
}

func ParseFlag(name string) (Shell, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return Unknown, nil
	case "bash":
		return Bash, nil
	case "zsh":
		return Zsh, nil
	case "pwsh", "powershell":
		return PowerShell, nil
	}
	return Unknown, fmt.Errorf("unsupported shell %q (valid: bash, zsh, pwsh)", name)
}

func Detect() Shell {
	if runtime.GOOS == "windows" {
		return PowerShell
	}
	base := filepath.Base(os.Getenv("SHELL"))
	switch base {
	case "zsh":
		return Zsh
	case "bash", "sh":
		return Bash
	case "pwsh", "powershell":
		return PowerShell
	}
	return Bash
}

func Resolve(flag string) (Shell, error) {
	s, err := ParseFlag(flag)
	if err != nil {
		return Unknown, err
	}
	if s == Unknown {
		s = Detect()
	}
	return s, nil
}

func ExportLine(sh Shell, path string) (string, error) {
	switch sh {
	case Bash, Zsh:
		return "export KUBECONFIG=" + posixQuote(path), nil
	case PowerShell:
		return "$env:KUBECONFIG = " + pwshQuote(path), nil
	}
	return "", fmt.Errorf("cannot emit export line for shell %s", sh)
}

func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func pwshQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
