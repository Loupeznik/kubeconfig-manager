# Roadmap

## Upcoming (v0.10.x maintenance)

Staying on the v0.10.x line for a while to shake out bugs and collect real-world feedback from the helm-guard + TUI import/merge work that shipped in v0.10.0. No new headline features are planned during this window; patch releases will focus on:

- Bug fixes surfaced from manual and production use.
- Follow-up polish from `.sisyphus/drafts/post-v0.10.0-review.md` items not yet addressed.
- CI / dependency hygiene (action bumps, goreleaser minor updates).

Larger planned features resume in v0.11.x — see **Long-term** below for the queue.

---

## Long-term

### Versioned documentation site via `mike`

The Pages site currently publishes the docs from the **latest release tag** (single version). Long-term we want a version switcher (dropdown showing v0.9.0, v0.9.1, v0.10.0, …, dev) via [**`mike`**](https://github.com/jimporter/mike).

Plan:

1. Add `mike` to `requirements-docs.txt` alongside `mkdocs-material`.
2. Add `extra.version.provider: mike` to `mkdocs.yml` — the Material theme renders the switcher automatically.
3. Extend `.github/workflows/docs.yml`:
   - On push to `master` → `mike deploy --push --update-aliases <date> dev`.
   - On tag push `v*` → `mike deploy --push --update-aliases <tag> latest`.
4. `mike` keeps all versions on the `gh-pages` branch; switch the Pages source from the GitHub Actions artifact to the `gh-pages` branch.

Proposed next feature target once the v0.10.x maintenance window closes.

### Cloud sync — pluggable state backends

State is already sync-friendly: stable-topology keys, `updated_at` timestamps, versioned schema. The `Store` interface in `internal/state` accepts alternative implementations without changes to the CLI or TUI.

Planned backends, in order of complexity:

1. **Git-backed** — point at a private repo, commit on every write. Zero server infra. Ships first.
2. **S3-compatible** — bucket + last-writer-wins via ETags. Works with Minio, R2, etc.
3. **Vault KV** — for teams that already have Vault.

No credentials ever leave the local kubeconfig files — sync is metadata only.

### Group-scoped alerts by tag

Instead of enabling alerts per-kubeconfig, apply them to every kubeconfig carrying a given tag (e.g. all `prod`-tagged). Add a `tag_policies:` section in the state file keyed by tag. Bigger design lift than the guard / sync items; most useful once cloud-sync lands and teams can share tag policies.

---

## Shipped

Items that were on this roadmap and have since landed, for history:

- **v0.10.0** — helm values-path guard (two scopes, multi-pattern list, path-token global fallback, default ON via tri-state); TUI parity for import + merge; `kcm doctor`, `kcm audit`, `kcm init` wizard; `--dry-run` on every mutating command; stable-hash state keys made default; major refactor (state / cli / tui split into per-feature files); multi-arch Docker images published to ghcr with cosign keyless signing.
- **v0.9.2** — fish shell support; `kcm context rename` / `kcm context delete` with TUI keybindings (`R`/`D`/`S`); `kcm starship` prompt integration; testscript end-to-end suite; testable `guard.Exec` via stub kubectl; alert-indicator cleanup in TUI detail view; `runtime/debug` build-info fallback for `go install`; docs site now deploys from the latest release tag.
- **v0.9.1** — dynamic shell completion; `kcm prune` for stale/orphaned state entries; CLI golden tests with `xdg.Reload` isolation; Node 24-capable GitHub Actions.
- **v0.9.0** — initial public release: TUI + CLI for kubeconfig files, tags, destructive-action guard, shell integration (bash/zsh/pwsh), import/split/merge, Apache 2.0 license, multi-platform release workflow.
