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
	Fish
)

func (s Shell) String() string {
	switch s {
	case Bash:
		return "bash"
	case Zsh:
		return "zsh"
	case PowerShell:
		return "pwsh"
	case Fish:
		return "fish"
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
	case "fish":
		return Fish, nil
	}
	return Unknown, fmt.Errorf("unsupported shell %q (valid: bash, zsh, pwsh, fish)", name)
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
	case "fish":
		return Fish
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
	case Fish:
		return "set -gx KUBECONFIG " + fishQuote(path), nil
	}
	return "", fmt.Errorf("cannot emit export line for shell %s", sh)
}

func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func pwshQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// fishQuote wraps in single quotes, escaping the only characters fish treats
// specially inside single-quoted strings: backslash and single-quote.
func fishQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}
