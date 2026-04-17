# Getting started

## Install

### From source
```sh
go install github.com/loupeznik/kubeconfig-manager/cmd/kubeconfig-manager@latest
```
The binary is named `kubeconfig-manager`. Link or alias it to `kcm` for brevity (the install-shell-hook below does this for you).

### Pre-built release
Grab a tarball from the [releases page](https://github.com/loupeznik/kubeconfig-manager/releases) and drop the `kcm` binary somewhere on your `PATH`.

## First run

With a few kubeconfigs in `~/.kube/`:

```sh
kcm list                    # table of files, context counts, tags, alerts
kcm show prod               # detail of one file (bare-name lookup + .yaml/.yml fallback)
kcm contexts                # contexts inside the default ~/.kube/config
```

All read-only commands respect `--dir <path>` to target a different directory.

## Install the shell hook

`kcm use` and the TUI print an `export KUBECONFIG=...` snippet; a shell function captures it.

```sh
kcm install-shell-hook          # auto-detects your shell from $SHELL
```

Supported shells: `bash`, `zsh`, `pwsh`. Restart your shell (or `source` the rc file) and:

```sh
kcm use prod                    # updates KUBECONFIG in this shell
kcm tui                         # interactive picker; press `x` to select and export
```

See [shell-integration.md](shell-integration.md) for details on the optional `kubectl` alias.

## Tag something

```sh
kcm tag add prod prod eu critical
kcm alert enable prod
kcm list                        # tags and ALERT flag now show
```

Metadata lives in `$XDG_CONFIG_HOME/kubeconfig-manager/config.yaml` and is keyed by the SHA-256 of each file's contents, so renames survive. See [state-file.md](state-file.md).

## Try the guard

With alerts enabled on `prod`, running a destructive verb through `kcm kubectl` prompts for confirmation:

```sh
export KUBECONFIG=~/.kube/prod.yaml
kcm kubectl delete pod mypod    # prompts: "Proceed with kubectl delete?"
```

If you'd like every `kubectl` invocation (not just `kcm kubectl`) to route through the guard, re-run the hook installer with `--alias-kubectl`. See [guard.md](guard.md).

## Where to go next

- [Shell integration deep-dive](shell-integration.md)
- [State file schema](state-file.md) — useful if you want to sync it across machines (content-hash keying makes this safe)
- [Import / split / merge](import-split-merge.md) — reorganizing kubeconfigs
