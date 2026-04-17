# Tags and alerts

Tags and alert policies are local metadata layered on top of each kubeconfig file. They live in `$XDG_CONFIG_HOME/kubeconfig-manager/config.yaml`, never inside the kubeconfig files themselves.

## Tags

```sh
kcm tag add prod prod eu critical         # add multiple tags
kcm tag remove prod eu                    # remove specific tags
kcm tag list                              # table of every file and its tags
kcm tag list prod                         # tags for one file
```

Tags are free-form, whitespace is trimmed, duplicates are ignored. They appear in `kcm list` as a comma-separated column and in the TUI as green badges next to each kubeconfig.

## Alerts

```sh
kcm alert enable prod                     # enables with sensible defaults
kcm alert show prod                       # current policy
kcm alert disable prod
```

Default policy on `enable`:

| Field | Default |
|---|---|
| `enabled` | `true` |
| `require_confirmation` | `true` |
| `confirm_cluster_name` | `false` |
| `blocked_verbs` | `delete, drain, cordon, uncordon, taint, replace, patch` |

Alerts only fire when you run `kubectl` **through** `kcm`:

- `kcm kubectl delete pod foo` — evaluated against the active kubeconfig's policy.
- `kubectl delete pod foo` (direct) — not intercepted unless you installed the shell hook with `--alias-kubectl`. See [shell-integration.md](shell-integration.md#opt-in-route-kubectl-through-the-guard).

## Custom blocked verbs

The default list covers the most common foot-guns, but you can edit the state file directly (see [state-file.md](state-file.md)) to customize `blocked_verbs` per kubeconfig. CLI commands for custom verb lists are not in v0.9 scope — open an issue if you want them.

## Metadata and renames

Entries are keyed by `sha256:<hex of file contents>`, not by filename. This means:

- `kcm rename prod.yaml prod-eu.yaml` moves the file on disk; the metadata automatically re-binds because the hash is unchanged.
- Copying the same kubeconfig to another machine yields the same hash, so cloud sync (deferred) will correctly unify metadata across machines.
- Editing the kubeconfig (e.g. rotating credentials) produces a new hash; the old metadata is orphaned. A `kcm prune` command for that is on the roadmap.
