# Roadmap

Features explicitly deferred out of v0.9, in rough priority order.

## Cloud sync

State is already sync-friendly: content-hash keys, `updated_at` timestamps, versioned schema. The `Store` interface in `internal/state` accepts alternative implementations without changes to the CLI or TUI.

Planned backends (in order of likelihood):

1. **Git-backed** — point at a private repo, commit on every write. Zero server infra.
2. **S3-compatible** — bucket + last-writer-wins via ETags. Works with Minio, R2, etc.
3. **Vault KV** — for teams that already have Vault.

No credentials ever leave the local kubeconfig files — sync is metadata only.

## Helm guard with values-path mismatch alert

When you run `kcm helm upgrade -f <path>/clusters/<name>/values.yaml ...`, `kcm` would perform a semantic fuzzy match between the path-derived cluster name and your active kubectl context. A mismatch (e.g. path says `k8s-test-01`, context is `k8s-prod-01`) would prompt for confirmation before executing.

**Opt-in in two scopes**, per-entry overriding global:

- **Global** (`helm_guard: { enabled: true, pattern: "clusters/{name}/values.yaml" }` at the state-file root) — applies to all kubeconfigs unless overridden.
- **Per-entry** (`entries[hash].helm_guard`) — enable/disable + pattern override for one kubeconfig; wins over global.

Resolution: per-entry > global > off (default off).

Reuses Phase 5's confirmation flow and the same opt-in shell-alias pattern as the `kubectl` alias.

## Fish shell support

Adding `fish` to the shell formatter is a handful of lines (different syntax for `set -x KUBECONFIG`). Deferred because the three included shells (bash, zsh, pwsh) cover the vast majority of users; revisit if requested.

## Stale-entry prune

When a kubeconfig's contents change (credential rotation), its hash changes and the old state entry becomes orphaned. A `kcm prune` command would list and optionally remove state entries whose `path_hint` no longer points to a file with the matching hash.

## Documentation site

The markdown in `docs/` is already structured for static-site rendering. Recommended framework: **[MkDocs](https://www.mkdocs.org/) with the [Material theme](https://squidfunk.github.io/mkdocs-material/)**.

Why MkDocs Material:
- De-facto standard for Kubernetes-adjacent CLI docs (Helm, Kustomize, ArgoCD, Velero, Flux, etc.).
- Builds from the same markdown already in `docs/` with minimal frontmatter.
- Built-in search, dark mode, versioning via `mike`.
- One-workflow deploy to GitHub Pages via `peaceiris/actions-gh-pages` or `mkdocs gh-deploy`.
- Python toolchain — a single `mkdocs.yml` plus a `pip install mkdocs-material`.

Migration plan when ready:

1. Add `mkdocs.yml` at repo root with `theme: material` and `nav:` entries mirroring `docs/README.md`.
2. Add `.github/workflows/docs.yml` that runs `mkdocs gh-deploy --force` on pushes to main (after a manual approval gate, per the project's CI/CD convention).
3. Enable GitHub Pages in repo settings, source = `gh-pages` branch.
4. Point `docs/` URLs at the hosted site; keep the raw markdown files in-tree so GitHub's repo browser keeps rendering them too.

Alternatives considered:
- **Docusaurus** — React-based, more flexible but heavier toolchain. Worth it if you want blog/versioning/custom React components.
- **Hugo + Docsy** — Kubernetes itself uses this; very capable but more opinionated and harder to skin.
- **Astro Starlight** — newer, fast, good DX. Fewer off-the-shelf integrations with the kube ecosystem.

For a CLI tool's reference docs, MkDocs Material hits the sweet spot.

## TUI parity with CLI ops

The TUI currently covers list, detail, tag-edit, rename, and alert toggle. Import/split/merge/use are CLI-only for v0.9. Adding a "File" menu with those ops is a modest Bubble Tea extension — the backing operations already live in `internal/kubeconfig/ops.go`.

## Group-scoped alerts by tag

Instead of enabling alerts per-kubeconfig, apply them to every kubeconfig carrying a given tag (e.g. all `prod`-tagged). This would mean a `tag_policies:` section in the state file keyed by tag. Bigger design lift than the other roadmap items; revisit after cloud sync lands.
