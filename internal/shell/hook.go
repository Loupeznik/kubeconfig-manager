package shell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	fenceStart = "# >>> kubeconfig-manager shell hook >>>"
	fenceEnd   = "# <<< kubeconfig-manager shell hook <<<"
)

type HookOptions struct {
	BinaryName   string
	AliasKubectl bool
	AliasHelm    bool
}

func (h HookOptions) binary() string {
	if h.BinaryName == "" {
		return "kubeconfig-manager"
	}
	return h.BinaryName
}

func RenderHook(sh Shell, opts HookOptions) (string, error) {
	switch sh {
	case Bash, Zsh:
		return renderPosixHook(sh, opts), nil
	case PowerShell:
		return renderPwshHook(opts), nil
	case Fish:
		return renderFishHook(opts), nil
	}
	return "", fmt.Errorf("cannot render hook for shell %s", sh)
}

func renderFishHook(opts HookOptions) string {
	var b strings.Builder
	b.WriteString(fenceStart)
	b.WriteString("\n# Managed by kubeconfig-manager. Do not edit between the fence markers.\n")
	b.WriteString("# To remove: kubeconfig-manager uninstall-shell-hook\n")
	fmt.Fprintf(&b, "function kcm\n")
	fmt.Fprintf(&b, "    switch $argv[1]\n")
	fmt.Fprintf(&b, "        case use tui\n")
	fmt.Fprintf(&b, "            eval (command %s $argv --shell=fish)\n", opts.binary())
	fmt.Fprintf(&b, "        case '*'\n")
	fmt.Fprintf(&b, "            command %s $argv\n", opts.binary())
	fmt.Fprintf(&b, "    end\n")
	fmt.Fprintf(&b, "end\n")
	if opts.AliasKubectl {
		fmt.Fprintf(&b, "alias kubectl \"command %s kubectl\"\n", opts.binary())
	}
	if opts.AliasHelm {
		fmt.Fprintf(&b, "alias helm \"command %s helm\"\n", opts.binary())
	}
	b.WriteString(fenceEnd)
	b.WriteString("\n")
	return b.String()
}

func renderPosixHook(sh Shell, opts HookOptions) string {
	var b strings.Builder
	b.WriteString(fenceStart)
	b.WriteString("\n# Managed by kubeconfig-manager. Do not edit between the fence markers.\n")
	b.WriteString("# To remove: kubeconfig-manager uninstall-shell-hook\n")
	fmt.Fprintf(&b, "kcm() {\n")
	fmt.Fprintf(&b, "    case \"$1\" in\n")
	fmt.Fprintf(&b, "        use|tui)\n")
	fmt.Fprintf(&b, "            eval \"$(command %s \"$@\" --shell=%s)\"\n", opts.binary(), sh)
	fmt.Fprintf(&b, "            ;;\n")
	fmt.Fprintf(&b, "        *)\n")
	fmt.Fprintf(&b, "            command %s \"$@\"\n", opts.binary())
	fmt.Fprintf(&b, "            ;;\n")
	fmt.Fprintf(&b, "    esac\n")
	fmt.Fprintf(&b, "}\n")
	if opts.AliasKubectl {
		fmt.Fprintf(&b, "alias kubectl='command %s kubectl'\n", opts.binary())
	}
	if opts.AliasHelm {
		fmt.Fprintf(&b, "alias helm='command %s helm'\n", opts.binary())
	}
	b.WriteString(fenceEnd)
	b.WriteString("\n")
	return b.String()
}

func renderPwshHook(opts HookOptions) string {
	var b strings.Builder
	b.WriteString(fenceStart)
	b.WriteString("\n# Managed by kubeconfig-manager. Do not edit between the fence markers.\n")
	b.WriteString("# To remove: kubeconfig-manager uninstall-shell-hook\n")
	fmt.Fprintf(&b, "function kcm {\n")
	fmt.Fprintf(&b, "    if ($args.Count -gt 0 -and ($args[0] -eq 'use' -or $args[0] -eq 'tui')) {\n")
	fmt.Fprintf(&b, "        $out = & %s @args --shell=pwsh\n", opts.binary())
	fmt.Fprintf(&b, "        if ($out) { Invoke-Expression $out }\n")
	fmt.Fprintf(&b, "    } else {\n")
	fmt.Fprintf(&b, "        & %s @args\n", opts.binary())
	fmt.Fprintf(&b, "    }\n")
	fmt.Fprintf(&b, "}\n")
	if opts.AliasKubectl {
		fmt.Fprintf(&b, "function kubectl { & %s kubectl @args }\n", opts.binary())
	}
	if opts.AliasHelm {
		fmt.Fprintf(&b, "function helm { & %s helm @args }\n", opts.binary())
	}
	b.WriteString(fenceEnd)
	b.WriteString("\n")
	return b.String()
}

func RCPath(sh Shell) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch sh {
	case Bash:
		return filepath.Join(home, ".bashrc"), nil
	case Zsh:
		return filepath.Join(home, ".zshrc"), nil
	case PowerShell:
		if runtime.GOOS == "windows" {
			return filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"), nil
		}
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), nil
	case Fish:
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	}
	return "", fmt.Errorf("unknown shell")
}

type InstallResult struct {
	RCPath  string
	Created bool
	Updated bool
}

func InstallHook(rcPath, hook string) (InstallResult, error) {
	if rcPath == "" {
		return InstallResult{}, errors.New("rcPath is empty")
	}
	if err := os.MkdirAll(filepath.Dir(rcPath), 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create rc dir: %w", err)
	}

	existing, err := os.ReadFile(rcPath)
	created := errors.Is(err, os.ErrNotExist)
	if err != nil && !created {
		return InstallResult{}, fmt.Errorf("read %s: %w", rcPath, err)
	}

	next, updated := replaceOrAppendBlock(string(existing), hook)
	if !created && !updated && string(existing) == next {
		return InstallResult{RCPath: rcPath}, nil
	}

	if err := atomicWrite(rcPath, []byte(next), 0o644); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{RCPath: rcPath, Created: created, Updated: updated}, nil
}

func UninstallHook(rcPath string) (removed bool, err error) {
	data, err := os.ReadFile(rcPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", rcPath, err)
	}

	next, removed := removeBlock(string(data))
	if !removed {
		return false, nil
	}

	if err := atomicWrite(rcPath, []byte(next), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func replaceOrAppendBlock(existing, block string) (string, bool) {
	start := strings.Index(existing, fenceStart)
	if start < 0 {
		trimmed := strings.TrimRight(existing, "\n")
		if trimmed == "" {
			return block, false
		}
		return trimmed + "\n\n" + block, false
	}
	endIdx := strings.Index(existing[start:], fenceEnd)
	if endIdx < 0 {
		return existing[:start] + block, true
	}
	endOfBlock := start + endIdx + len(fenceEnd)
	if endOfBlock < len(existing) && existing[endOfBlock] == '\n' {
		endOfBlock++
	}
	return existing[:start] + block + existing[endOfBlock:], true
}

func removeBlock(existing string) (string, bool) {
	start := strings.Index(existing, fenceStart)
	if start < 0 {
		return existing, false
	}
	endIdx := strings.Index(existing[start:], fenceEnd)
	if endIdx < 0 {
		return existing, false
	}
	endOfBlock := start + endIdx + len(fenceEnd)
	if endOfBlock < len(existing) && existing[endOfBlock] == '\n' {
		endOfBlock++
	}
	prefix := existing[:start]
	suffix := existing[endOfBlock:]
	prefix = strings.TrimRight(prefix, "\n")
	result := prefix
	if suffix != "" {
		if prefix != "" {
			result += "\n"
		}
		result += suffix
	} else if prefix != "" {
		result += "\n"
	}
	return result, true
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}
