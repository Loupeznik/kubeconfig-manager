# kubeconfig-manager documentation

`kcm` is a CLI + TUI for managing local kubeconfig files and kubectl contexts, with tags, destructive-action guardrails, and first-class shell integration.

## Table of contents

### Guides
- [Getting started](getting-started.md) — install, shell hook, first run
- [Shell integration](shell-integration.md) — `use`, `tui`, `install-shell-hook`, optional `kubectl` alias
- [Tags and alerts](tags-and-alerts.md) — how metadata attaches to kubeconfigs
- [Destructive-action guard](guard.md) — how `kcm kubectl` intercepts dangerous verbs
- [Helm values-path guard](helm-guard.md) — catch values-file / context mismatches before `helm upgrade` ruins your day
- [Import, split, merge](import-split-merge.md) — reorganizing kubeconfig files
- [State file](state-file.md) — schema, storage location, sync-readiness
- [Architecture](architecture.md) — package layout and design decisions
- [Roadmap](roadmap.md) — deferred features (helm guard, cloud sync, fish shell)

### CLI reference
Auto-generated from the Cobra command tree. Regenerate with `go run scripts/gendocs.go`.

- [`kcm`](cli/kcm.md) — root command
- [`kcm list`](cli/kcm_list.md)
- [`kcm show`](cli/kcm_show.md)
- [`kcm contexts`](cli/kcm_contexts.md)
- [`kcm use`](cli/kcm_use.md)
- [`kcm tui`](cli/kcm_tui.md)
- [`kcm tag`](cli/kcm_tag.md)
- [`kcm alert`](cli/kcm_alert.md)
- [`kcm rename`](cli/kcm_rename.md)
- [`kcm import`](cli/kcm_import.md)
- [`kcm split`](cli/kcm_split.md)
- [`kcm merge`](cli/kcm_merge.md)
- [`kcm kubectl`](cli/kcm_kubectl.md)
- [`kcm install-shell-hook`](cli/kcm_install-shell-hook.md)
- [`kcm uninstall-shell-hook`](cli/kcm_uninstall-shell-hook.md)

### Man pages
Installable man pages are generated into the `man/` directory alongside the markdown (`man/kcm.1`, `man/kcm-list.1`, …). After `goreleaser` publishes a release, these are packaged into the downloadable archives.

## Hosting documentation

When you're ready to publish this at a real URL, the recommended framework is **MkDocs with the Material theme** — it's the de-facto standard for Kubernetes-adjacent CLI tools (Helm, Kustomize, ArgoCD, Velero all use it), builds from the same markdown in `docs/`, and deploys to GitHub Pages with a single GitHub Action. See [roadmap](roadmap.md#documentation-site) for the concrete migration plan.
