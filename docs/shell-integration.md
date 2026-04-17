# Shell integration

`kcm use` and `kcm tui` print an `export KUBECONFIG=...` line to stdout (the TUI writes its interface to stderr to keep stdout clean). A shim function `eval`s that line so your current shell picks up the new value.

## Installing the hook

```sh
kubeconfig-manager install-shell-hook            # auto-detect
kubeconfig-manager install-shell-hook --shell=zsh
kubeconfig-manager install-shell-hook --shell=pwsh
```

| Shell | Default rc file |
|---|---|
| `bash` | `~/.bashrc` |
| `zsh` | `~/.zshrc` |
| `pwsh` | `~/.config/powershell/Microsoft.PowerShell_profile.ps1` (unix), `~/Documents/PowerShell/Microsoft.PowerShell_profile.ps1` (Windows) |

Pass `--rc <path>` to target a different file.

The installer writes a fenced block — idempotent, safe to re-run:

```sh
# >>> kubeconfig-manager shell hook >>>
kcm() {
    case "$1" in
        use|tui)
            eval "$(command kubeconfig-manager "$@" --shell=zsh)"
            ;;
        *)
            command kubeconfig-manager "$@"
            ;;
    esac
}
# <<< kubeconfig-manager shell hook <<<
```

Re-running `install-shell-hook` replaces the block in place; surrounding rc content is preserved.

## Uninstalling

```sh
kubeconfig-manager uninstall-shell-hook
```

Removes only the fenced block. You can also remove it manually — the fence markers are stable.

## kubectl alias (on by default)

By default, `install-shell-hook` writes an additional line inside the same fence block:

```sh
alias kubectl='command kubeconfig-manager kubectl'
```

This ensures destructive-action alerts fire for plain `kubectl delete|drain|...` invocations too, not just `kcm kubectl ...`. Without the alias, running `kubectl delete` directly would bypass the guard entirely.

**Trade-offs:**

- One extra process on every `kubectl` invocation (negligible, ~1ms).
- Alert confirmations may interrupt scripts — disable alerts on the relevant kubeconfig (`kcm alert disable <file>`) or run non-interactive workloads through the plain binary path (`command kubectl ...`, which bypasses the alias).
- Removing the alias is one command: `kubeconfig-manager uninstall-shell-hook`, or edit the rc file and delete the fenced block.

## Opting out of the kubectl alias

If you'd rather only get alerts through explicit `kcm kubectl ...` invocations, pass `--no-alias-kubectl`:

```sh
kubeconfig-manager install-shell-hook --no-alias-kubectl
```

The installed fence block will only contain the `kcm()` function, not the `kubectl` alias. **Note:** this means `kubectl delete` and similar destructive commands will run without any guardrail, even when you've enabled alerts on the active kubeconfig.

## How shell detection works

`kcm` resolves the target shell in this order:

1. `--shell=<name>` flag if passed (valid: `bash`, `zsh`, `pwsh`).
2. On Windows, default to `pwsh`.
3. Otherwise, inspect `$SHELL`:
   - `zsh` → Zsh formatter
   - `bash`, `sh` → Bash formatter
   - `pwsh`, `powershell` → PowerShell formatter
4. Fallback: Bash.

## How `eval` works safely

The TUI writes its UI to stderr via `tea.WithOutput(os.Stderr)`, leaving stdout empty until the user presses `x` to select — at which point the single-line export is printed. If the user quits with `q`, stdout stays empty and the surrounding `eval $(...)` does nothing.

Paths are quoted with POSIX-safe single-quote escaping for bash/zsh and PowerShell single-quote doubling for pwsh, so filenames containing apostrophes don't break the emitted line.
