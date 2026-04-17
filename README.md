# kubeconfig-manager

`kcm` — a TUI + CLI for managing local kubeconfig files and kubectl contexts, with tagging and destructive-action guardrails.

## Status

Early scaffold. Subcommands are registered but not yet implemented. See [`.sisyphus/drafts/kubeconfig-manager-bootstrap.md`](.sisyphus/drafts/kubeconfig-manager-bootstrap.md) for the full analysis and phased implementation plan.

## Features (planned v0.1)

- Browse and manage kubeconfig files in a configurable directory (default `~/.kube`).
- Attach tags and a display name to each kubeconfig (local state, content-hash keyed, sync-ready).
- Configure destructive-action alerts per kubeconfig (warn on `delete`, `drain`, `cordon`, etc. when invoked via `kcm kubectl`).
- Switch kubeconfigs by printing an `export KUBECONFIG=...` line for `bash`, `zsh`, and `powershell`.
- Import, split, and merge kubeconfigs against the default `~/.kube/config`.
- Bubble Tea TUI for all of the above.

## Install

```sh
go install github.com/loupeznik/kubeconfig-manager/cmd/kubeconfig-manager@latest
```

The binary installs as `kubeconfig-manager`; symlink or alias to `kcm` for the shorter name.

## Usage

```sh
kcm --help
kcm list
kcm use prod-eu
kcm tui
```

See `kcm <command> --help` for per-command details.

## Shell integration

`kcm use` and `kcm tui` print an `export KUBECONFIG=...` snippet to stdout (TUI rendering goes to stderr). To have the snippet actually update your current shell, you need a wrapper function — `kcm install-shell-hook` writes one for you.

### Install

```sh
kubeconfig-manager install-shell-hook            # auto-detects your shell from $SHELL
kubeconfig-manager install-shell-hook --shell=zsh
kubeconfig-manager install-shell-hook --shell=pwsh
```

Supported first-class shells: `bash`, `zsh`, `pwsh`. The installer writes a fenced block into `~/.zshrc`, `~/.bashrc`, or the PowerShell profile (`~/.config/powershell/Microsoft.PowerShell_profile.ps1` on unix, `Documents/PowerShell/...` on Windows). Reinstalling is idempotent — the block is replaced in place, not appended. Pass `--rc <path>` to target a custom file.

After install, restart your shell or `source` the rc file. Then:

```sh
kcm use prod-eu     # updates KUBECONFIG in this shell
kcm tui             # interactive picker; pressing `x` sets KUBECONFIG
```

### Optional: alias kubectl through the guard

Pass `--alias-kubectl` to additionally alias `kubectl` so every invocation routes through `kcm kubectl`, which enforces the per-kubeconfig destructive-action alerts:

```sh
kubeconfig-manager install-shell-hook --alias-kubectl
```

**Trade-offs:** adds one process hop to every `kubectl` invocation, and alert confirmations intercept destructive commands (`delete`, `drain`, `cordon`, etc.) when the active kubeconfig has alerts enabled. Opt-in by design.

### Uninstall

```sh
kubeconfig-manager uninstall-shell-hook
```

Removes the fenced block; the rest of your rc file is preserved. You can also edit the rc file manually — the block is clearly marked with `# >>> kubeconfig-manager shell hook >>>` / `# <<< kubeconfig-manager shell hook <<<` fences.

## License

Apache 2.0. See [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
