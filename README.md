# kubeconfig-manager

[![ci](https://github.com/loupeznik/kubeconfig-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/loupeznik/kubeconfig-manager/actions/workflows/ci.yml)
[![release](https://github.com/loupeznik/kubeconfig-manager/actions/workflows/release.yml/badge.svg)](https://github.com/loupeznik/kubeconfig-manager/actions/workflows/release.yml)

`kcm` — a TUI + CLI for managing local kubeconfig files and kubectl contexts, with tagging and destructive-action guardrails.

Full docs live in [`docs/`](docs/README.md) — start with [Getting started](docs/getting-started.md).

## Features

- Browse kubeconfig files in a configurable directory (default `~/.kube`).
- Attach tags and alert policies to each kubeconfig — metadata is keyed by the file's SHA-256 so it survives renames and is ready for cloud sync.
- Destructive-action guard: `kcm kubectl delete|drain|cordon|...` prompts for confirmation on flagged kubeconfigs.
- Switch kubeconfigs with a shell-appropriate `export KUBECONFIG=...` line (bash / zsh / pwsh).
- Import, split, and merge kubeconfigs via `clientcmd` — atomic writes, no half-written files.
- Bubble Tea TUI with list, detail, tag editor, rename, and alert toggle.

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

## Documentation

- [User docs index](docs/README.md)
- [Getting started](docs/getting-started.md)
- [Shell integration](docs/shell-integration.md)
- [Destructive-action guard](docs/guard.md)
- [State file schema](docs/state-file.md)
- [Architecture](docs/architecture.md)
- [Roadmap](docs/roadmap.md) (deferred features + docs-site framework recommendation)

CLI reference and man pages are regenerated from the command tree:

```sh
go run scripts/gendocs.go
```

## Contributing

1. `scripts/build.sh` — builds `./bin/kcm` with version info baked in.
2. `scripts/lint.sh` — gofmt, `go vet`, `golangci-lint`.
3. `go test ./...` — 66 unit tests across kubeconfig ops, state, shell, and guard packages.
4. Run the binary against `./.temp/` rather than the real `~/.kube/` during development (seed mock kubeconfigs there; it's gitignored).

## License

Apache 2.0. See [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
