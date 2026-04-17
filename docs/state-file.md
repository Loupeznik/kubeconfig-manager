# State file

`kcm` keeps all its local metadata in a single YAML file:

- **Unix:** `$XDG_CONFIG_HOME/kubeconfig-manager/config.yaml` (defaults to `~/.config/kubeconfig-manager/config.yaml`)
- **Windows:** `%APPDATA%\kubeconfig-manager\config.yaml`

File mode is `0o600` on unix. Writes are atomic (temp + rename) and serialized via a `flock` on `config.yaml.lock` so concurrent `kcm` invocations don't corrupt it.

## Schema (v1)

```yaml
version: 1
kubeconfig_dir: /home/you/.kube           # optional; informational
entries:
  sha256:9f04fe2c...:                     # key = SHA-256 of the kubeconfig file's bytes
    path_hint: prod.yaml                  # last-known filename, informational
    display_name: "Prod EU"               # future: TUI label (v0.1 stores but doesn't edit)
    tags: [prod, eu, critical]
    alerts:
      enabled: true
      require_confirmation: true
      confirm_cluster_name: false
      blocked_verbs: [delete, drain, cordon, uncordon, taint, replace, patch]
    updated_at: 2026-04-17T12:00:00Z
  sha256:218b0740...:
    path_hint: staging.yaml
    tags: [staging]
    updated_at: 2026-04-17T12:05:00Z
```

### Why content-hash keys?

Because metadata follows the file, not its location:

- `kcm rename prod.yaml prod-eu.yaml` — file moves, hash stays identical, metadata still binds.
- The same kubeconfig copied to another machine produces the same hash, so future cloud-sync will correctly unify entries across hosts.
- Editing the kubeconfig (e.g. token rotation) changes the hash. The old entry is orphaned — by design, so rotated credentials don't silently inherit a dangerous alert policy.

### Versioning

`version: 1` is the current schema. `kcm` refuses to load a state file with a higher version — forward-compatibility comes via an explicit migration, not silent best-effort parsing.

If you ever need to reset metadata, delete the file; `kcm` recreates it on next write.

## Concurrency

Each `kcm` invocation that mutates state goes through `Mutate()`, which:

1. Creates the config directory if missing.
2. Acquires an exclusive `flock` on `config.yaml.lock` with a small retry loop.
3. Re-reads the file inside the lock.
4. Applies the caller's change function.
5. Writes atomically (temp + rename) and releases the lock.

This means two `kcm` commands racing on different shells won't lose updates. The lock file is intentionally separate from the config file so editors watching the config aren't disturbed.

## Future: cloud sync

The `Store` interface in `internal/state` is designed to accept additional implementations (S3, Git-backed, Vault) without changing any CLI or TUI code. The content-hash keying and `updated_at` per entry are the two pieces you need for last-writer-wins sync across machines. See [roadmap.md](roadmap.md#cloud-sync).
