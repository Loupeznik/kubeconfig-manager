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

`kcm use` prints an export snippet — source it with a shim function in your shell rc:

```sh
# bash / zsh
kcm() {
  if [[ "$1" == "use" ]]; then
    eval "$(command kubeconfig-manager "$@")"
  else
    command kubeconfig-manager "$@"
  fi
}
```

`kcm install-shell-hook` will write this for you. Pass `--alias-kubectl` to additionally alias `kubectl` through the destructive-action guard (opt-in — see docs for trade-offs and uninstall steps).

## License

Apache 2.0. See [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
