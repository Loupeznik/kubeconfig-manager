# Roadmap

## Upcoming (v0.10.x)

### Helm guard with values-path mismatch alert

Extend the destructive-action guard concept to `helm`. When enabled, `kcm helm upgrade -f <path>/clusters/<name>/values.yaml ...` performs a semantic fuzzy match between the values-file path component and the active kubectl context. On significant mismatch (e.g. path says `k8s-test-01`, active context is `k8s-prod-01`), prompt for confirmation before executing.

**Opt-in, two scopes:**

- **Global** (`helm_guard.enabled: true` at state-file root) — applies to every kubeconfig unless overridden.
- **Per-entry** (`entries[hash].helm_guard`) — enable/disable + pattern override for one kubeconfig; wins over global.

Resolution: per-entry > global > off (default off).

Configurable path-to-name pattern so different repo layouts are supported (default `clusters/{name}/`).

Reuses Phase 5's confirmation flow and the same opt-in shell-alias pattern as the `kubectl` alias.

### TUI parity with CLI ops (import + merge)

The TUI covers list, detail, tag, alert, rename, split, and use. Import and merge are still CLI-only. Add in-TUI flows:

- List view `i` → prompt for a source kubeconfig path, merge into the highlighted file (or `~/.kube/config` if nothing selected).
- List view `m` → prompt for a second source path + output filename, merge via `kubeconfig.Merge`.

---

## Long-term

### Cloud sync — pluggable state backends

State is already sync-friendly: stable-topology keys, `updated_at` timestamps, versioned schema. The `Store` interface in `internal/state` accepts alternative implementations without changes to the CLI or TUI.

Planned backends, in order of complexity:

1. **Git-backed** — point at a private repo, commit on every write. Zero server infra. Ships first.
2. **S3-compatible** — bucket + last-writer-wins via ETags. Works with Minio, R2, etc.
3. **Vault KV** — for teams that already have Vault.

No credentials ever leave the local kubeconfig files — sync is metadata only.

### Versioned documentation site via `mike`

The Pages site currently publishes the docs from the **latest release tag** (single version). Long-term we want a version switcher (dropdown showing v0.9.0, v0.9.1, v0.10.0, …, dev) via [**`mike`**](https://github.com/jimporter/mike).

Plan:

1. Add `mike` to `requirements-docs.txt` alongside `mkdocs-material`.
2. Add `extra.version.provider: mike` to `mkdocs.yml` — the Material theme renders the switcher automatically.
3. Extend `.github/workflows/docs.yml`:
   - On push to `master` → `mike deploy --push --update-aliases <date> dev`.
   - On tag push `v*` → `mike deploy --push --update-aliases <tag> latest`.
4. `mike` keeps all versions on the `gh-pages` branch; switch the Pages source from the GitHub Actions artifact to the `gh-pages` branch.

### Group-scoped alerts by tag

Instead of enabling alerts per-kubeconfig, apply them to every kubeconfig carrying a given tag (e.g. all `prod`-tagged). Add a `tag_policies:` section in the state file keyed by tag. Bigger design lift than the guard / sync items; most useful once cloud-sync lands and teams can share tag policies.

---

## Shipped

Items that were on this roadmap and have since landed, for history:

- **v0.9.0** — initial public release: TUI + CLI for kubeconfig files, tags, destructive-action guard, shell integration (bash/zsh/pwsh), import/split/merge, Apache 2.0 license, multi-platform release workflow.
- **v0.9.1** — dynamic shell completion; `kcm prune` for stale/orphaned state entries; CLI golden tests with `xdg.Reload` isolation; Node 24-capable GitHub Actions.
- **v0.9.2** — fish shell support; `kcm context rename` / `kcm context delete` with TUI keybindings (`R`/`D`/`S`); `kcm starship` prompt integration; testscript end-to-end suite; testable `guard.Exec` via stub kubectl; alert-indicator cleanup in TUI detail view; `runtime/debug` build-info fallback for `go install`; docs site now deploys from the latest release tag.
