# Import, split, merge

Reorganizing kubeconfig files across the `~/.kube/` directory. All three commands use the upstream `clientcmd` library for parsing and write output atomically.

## import

Merge a new kubeconfig into your default config (or another destination).

```sh
kcm import ~/Downloads/new-cluster.yaml                        # merges into ~/.kube/config
kcm import new.yaml --into ~/.kube/staging.yaml
kcm import new.yaml --on-conflict=skip                         # keep destination on collision
kcm import new.yaml --on-conflict=overwrite                    # source wins on collision
```

Destination is auto-created if missing. Current-context of the destination is preserved unless it was unset, in which case the source's current-context is adopted.

## split

Pull a single context (with its cluster and user) into its own file.

```sh
kcm split prod-eu ~/.kube/prod-eu.yaml                          # from ~/.kube/config
kcm split staging staging.yaml --from ~/.kube/multi.yaml
kcm split old-ctx /tmp/old.yaml --remove                        # also delete from source
```

The output file's `current-context` is set to the extracted context. With `--remove`, the source is rewritten with that context (and any cluster/user references no longer used by other contexts) pruned. The destination must not exist — split refuses to overwrite.

## merge

Explicitly combine two files into a new one.

```sh
kcm merge a.yaml b.yaml merged.yaml
kcm merge a.yaml b.yaml merged.yaml --on-conflict=skip --force  # force overwrites existing merged.yaml
```

Collision policies (applies to clusters, users, and contexts independently):

| Policy | Behavior |
|---|---|
| `error` (default) | Refuse; list every collision so you can resolve them manually. |
| `skip` | Keep `a`'s version on collision; drop `b`'s. |
| `overwrite` | `b` wins on collision. |

`a`'s current-context is preserved if set; otherwise `b`'s.

## Atomicity and safety

All writes go through `clientcmd.WriteToFile`, which creates a temp file and atomically renames — your target file is never left in a half-written state. File mode is set to `0o600`.

## When to use which

- **import** when you've received a new kubeconfig and want it in your main file without renaming it.
- **split** when `~/.kube/config` has grown too crowded and you want to file a cluster out into its own file (which `kcm use` can then target by name).
- **merge** for one-off programmatic composition where neither of the above fits.

If you'd rather keep clusters in separate files and let `$KUBECONFIG` handle the union at runtime, use `kcm use` instead of merging. The colon-separated `$KUBECONFIG` approach works out of the box with the guard, too — each path is hashed and checked independently.
