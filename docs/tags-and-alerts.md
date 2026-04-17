# Tags and alerts

Tags and alert policies are local metadata layered on top of each kubeconfig file. They live in `$XDG_CONFIG_HOME/kubeconfig-manager/config.yaml`, never inside the kubeconfig files themselves.

## Tags

Tags are drawn from a **global palette** you maintain with `kcm tag palette`. Only tags in the palette can be assigned to kubeconfigs (unless you pass `--allow-new`, which auto-adds them). The palette fuels the TUI's multi-select picker and gives CLI assignments a typo-safe allow-list.

### Managing the palette

```sh
kcm tag palette list                              # show current palette
kcm tag palette add prod staging dev eu us        # add tags to the palette
kcm tag palette remove dev                        # remove from palette (also scrubs from every entry)
```

When the palette is empty, `kcm` bootstraps it from any tags already attached to entries on first access. You don't need to re-declare existing tags after upgrading.

### Assigning tags

```sh
kcm tag add prod prod eu critical                 # file-level
kcm tag add prod critical --context prod-eu       # per-context
kcm tag remove prod eu                            # file-level
kcm tag remove prod critical --context prod-eu    # per-context
kcm tag list                                      # every file with file+context rows
kcm tag list prod                                 # tags for one file (file + per-context)
kcm tag list prod --context prod-eu               # effective tags for a specific context
```

Unknown tags are rejected with a helpful error:

```
tag(s) not in palette: some-unknown-tag (run 'kcm tag palette add some-unknown-tag' first, or pass --allow-new)
```

Use `--allow-new` once to add ad-hoc tags on the fly — they land in the palette for future use.

Tags are whitespace-trimmed and deduplicated. They appear in `kcm list` as a column and in the TUI as green badges next to each kubeconfig, with cyan badges on the per-context view.

### TUI tag editor

With a populated palette, pressing `t` in the TUI opens a multi-select picker of palette tags:

- `space` toggles the cursor row
- `a` selects all, `n` selects none
- `↵` saves, `esc` cancels

If the palette is empty, the editor falls back to a comma-separated text input and suggests populating the palette.

## Alerts

Alerts can be set at two scopes, with per-context overrides winning over file-level policy. See [state-file.md](state-file.md#per-context-alert-resolution) for the resolution order.

```sh
# File-level (applies to every context in the file)
kcm alert enable prod                     # enables with sensible defaults
kcm alert show prod                       # shows file-level + any per-context overrides
kcm alert disable prod

# Per-context (overrides file-level for that one context)
kcm alert enable prod --context prod-eu
kcm alert disable prod --context prod-us  # explicitly disabled for this context only
kcm alert show prod --context prod-eu     # resolved policy for that context
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
