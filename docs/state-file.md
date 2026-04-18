# State file

`kcm` keeps all its local metadata in a single YAML file:

- **Unix:** `$XDG_CONFIG_HOME/kubeconfig-manager/config.yaml` (defaults to `~/.config/kubeconfig-manager/config.yaml`)
- **Windows:** `%APPDATA%\kubeconfig-manager\config.yaml`

File mode is `0o600` on unix. Writes are atomic (temp + rename) and serialized via a `flock` on `config.yaml.lock` so concurrent `kcm` invocations don't corrupt it.

## Schema (v1)

```yaml
version: 1
kubeconfig_dir: /home/you/.kube           # optional; informational
available_tags:                           # global tag palette (allow-list for tag assignment)
  - prod
  - staging
  - dev
  - eu
  - us
  - critical

helm_guard:                               # global helm values-path / context mismatch detector
  enabled: true
  patterns:                               # ordered list; first match wins
    - "clusters/{name}/"
    - "environments/{name}/"
  global_fallback: true                   # if no pattern matches, tokenize the raw path and compare
  env_tokens: [prod, production, staging, stg, stage, dev, test, qa, sandbox]

entries:
  sha256:9f04fe2c...:                     # key = stable-topology SHA-256 of the kubeconfig
    path_hint: prod.yaml                  # last-known filename, informational
    display_name: "Prod EU"               # future: TUI label (stored but not yet edited in CLI)
    tags: [prod, eu, critical]
    alerts:                               # file-level (applies to every context in the file)
      enabled: true
      require_confirmation: true
      confirm_cluster_name: false
      blocked_verbs: [delete, drain, cordon, uncordon, taint, replace, patch]
    context_alerts:                       # per-context override map (takes precedence over file-level)
      prod-eu:
        enabled: true
        confirm_cluster_name: true        # stricter policy for this one context
        blocked_verbs: [delete, drain, patch, apply]
      prod-us:
        enabled: false                    # explicitly disabled for this context even though file-level is on
    context_tags:                         # per-context additions on top of file-level tags
      prod-eu: [eu-primary]
    helm_guard:                           # optional per-entry helm-guard override
      enabled: false                      # explicit disable overrides the global policy for this entry
    updated_at: 2026-04-17T12:00:00Z

  sha256:218b0740...:
    path_hint: staging.yaml
    tags: [staging]
    helm_guard:                           # override just the pattern list for this one kubeconfig
      enabled: true
      patterns: ["legacy/{name}.yaml"]
    updated_at: 2026-04-17T12:05:00Z
```

### Per-context alert resolution

For a given kubectl invocation, `kcm` resolves the active context (from `--context <name>` on the args, or the kubeconfig's `current-context`) and then picks the alert policy in this order:

1. `entries[hash].context_alerts[<active-context>]` if present — the per-context policy wins, even if it sets `enabled: false` to explicitly suppress a file-level policy.
2. Otherwise, `entries[hash].alerts` — the file-level policy applies.
3. Otherwise, no alert fires.

### helm-guard resolution

The helm values-path guard has its own two-scope resolution:

1. `entries[hash].helm_guard` if present — per-entry policy. A nil/absent field inherits the global block. A struct with `enabled: false` explicitly suppresses the global policy for this entry.
2. Otherwise, the root `helm_guard` block. Its `patterns` list is tried in order, first match wins. When none match and `global_fallback: true`, the raw values-file path is tokenized and compared directly against the active context/cluster + environment tokens.
3. Otherwise, the guard is off.

`global_fallback` is OR-merged: either per-entry OR global being true enables the fallback. There's no tri-state, so a per-entry explicit `false` doesn't suppress the global fallback — if you need that, disable the entry entirely.

**Legacy single-pattern field.** State files written by v0.10.x used a scalar `pattern: "foo"` field instead of `patterns: [...]`. kcm still reads that form on load and migrates it into `patterns` on first save — no user action required.

### Why content-hash keys?

Because metadata follows the file, not its location:

- `kcm rename prod.yaml prod-eu.yaml` — file moves, hash stays identical, metadata still binds.
- `kubectl config use-context prod-us` flipping `current-context` — the stable hash ignores that flip, so metadata survives.
- The same kubeconfig copied to another machine produces the same hash, so future cloud-sync will correctly unify entries across hosts.

Since v0.9.2, the key is specifically the **stable topology hash** — SHA-256 of the kubeconfig's logical clusters/users/contexts, ignoring ordering, whitespace, and the volatile `current-context` pointer. A content-hash fallback read ensures old state files from v0.8.x load and rekey on first mutation.

Rotating credentials (e.g. new cluster certificate data) changes the topology and produces a new stable hash. The old entry is orphaned — by design, so rotated credentials don't silently inherit a dangerous alert policy. `kcm prune` lists and removes orphans.

### Versioning

`version: 1` is the current schema. `kcm` refuses to load a state file with a higher version — forward-compatibility comes via an explicit migration, not silent best-effort parsing.

Unknown top-level keys are tolerated (yaml.v3 non-strict mode) so users can hand-edit comments or future-proofed fields without kcm choking on them.

If you ever need to reset metadata, delete the file; `kcm` recreates it on next write.

## Audit log

`kcm` also appends one line per guard prompt (kubectl or helm) to a separate log:

- **Unix:** `$XDG_DATA_HOME/kubeconfig-manager/audit.log`
- **Windows:** `%LOCALAPPDATA%\kubeconfig-manager\audit.log`

Format is `timestamp key=value ...` with single-quoting for values that contain whitespace. Inspect with `kcm audit` or `grep context=prod-eu audit.log`.

## Concurrency

Each `kcm` invocation that mutates state goes through `Mutate()`, which:

1. Creates the config directory if missing.
2. Acquires an exclusive `flock` on `config.yaml.lock` with a small retry loop.
3. Re-reads the file inside the lock.
4. Applies the caller's change function.
5. Writes atomically (temp + rename) and releases the lock.

This means two `kcm` commands racing on different shells won't lose updates. The lock file is intentionally separate from the config file so editors watching the config aren't disturbed.

## Future: cloud sync

The `Store` interface in `internal/state` is designed to accept additional implementations (S3, Git-backed, Vault) without changing any CLI or TUI code. The stable-hash keying and `updated_at` per entry are the two pieces you need for last-writer-wins sync across machines. See [roadmap.md](roadmap.md#cloud-sync).
