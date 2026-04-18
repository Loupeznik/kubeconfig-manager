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

## Dynamic shell completion

Cobra already ships static completion via `kcm completion <bash|zsh|pwsh>` (fang wires this up automatically), but we can do better. Wire `cobra.ValidArgsFunction` on:

- `kcm use <TAB>` — complete with kubeconfig filenames in the managed directory.
- `kcm show <TAB>`, `kcm tag add/remove <TAB>`, `kcm alert enable/disable/show <TAB>`, `kcm rename <TAB>`, `kcm split <TAB>` — same.
- `kcm alert enable <file> --context <TAB>` — complete with context names parsed from that file.
- `kcm contexts --file <TAB>` — same.

Implementation sketch: each completion function uses `kubeconfig.ScanDir()` + the resolved `--dir` flag value to produce names, and per-context completions additionally call `kubeconfig.Load()` to enumerate `Contexts`. Results are cached in-process for a single command invocation.

One concrete payoff: `kcm alert enable prod --context <TAB><TAB>` should list the contexts actually present in `prod.yaml`, so the user can't typo a context name into the state file.

## Fish shell support

Adding `fish` to the shell formatter is a handful of lines (different syntax for `set -x KUBECONFIG`). Deferred because the three included shells (bash, zsh, pwsh) cover the vast majority of users; revisit if requested.

## Integration test suite

v0.9 ships with unit tests (~70 test cases across `shell`, `state`, `kubeconfig`, and `guard`) plus light cross-package tests that hit the real filesystem. No CLI-level or end-to-end coverage yet. Planned for v0.9.x:

- **CLI golden tests** (`internal/cli/*_test.go`) — instantiate `cli.NewRootCmd()`, feed `args`, capture `stdout`/`stderr`, assert against golden files. Fastest path to high-value coverage — the command tree is where most user-facing behavior lives and is currently untested.
- **End-to-end binary tests** using [`rsc.io/script`](https://pkg.go.dev/rsc.io/script/scripttest) or the built-in [`testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript). Drives the compiled `kcm` binary through scripted `.txt` scenarios covering the shell-hook flow, `kcm use` → `eval`, `kcm tag palette` lifecycle, and the `kcm kubectl` guard path with a fake `kubectl`.
- **Guard exec path** — currently the verb-detection and policy-resolution are covered, but `guard.Exec` (the actual `kubectl` spawn + stdin/stdout/exit-code passthrough) has no test. A small integration test using a stub `kubectl` binary can close this.

**Not planned:** TUI snapshot tests via `teatest`. Manual TUI verification has been reliable so far; the ROI of snapshot tests against a fast-moving UI surface isn't worth the flake budget right now.

## Stale-entry prune

When a kubeconfig's contents change (credential rotation), its hash changes and the old state entry becomes orphaned. A `kcm prune` command would list and optionally remove state entries whose `path_hint` no longer points to a file with the matching hash.

## Rename and delete contexts within a kubeconfig

Today `kcm` can split a context out into its own file (via `kcm split`) or remove one from its source file during that operation, but there's no direct "rename this context" or "delete this context" primitive. Add both in the CLI and the TUI:

- **CLI:** `kcm context rename <file> <old> <new>` and `kcm context delete <file> <name>` (or similar). Mutation goes through `clientcmd.WriteToFile` for atomic writes; references in other contexts/state are updated or scrubbed.
- **TUI:** from the detail view of a kubeconfig, add `R` to rename the highlighted context and `D` (capital, destructive) to delete it with a confirmation modal. Updates `current-context` if the active one was affected.

State handling: per-context tags and alerts attached to the old name should follow the rename (move under the new key) or be dropped on delete.

## TUI parity with CLI ops

The TUI currently covers list, detail, tag-edit, rename, and alert toggle. Import/split/merge/use are CLI-only for v0.9. Adding a "File" menu with those ops is a modest Bubble Tea extension — the backing operations already live in `internal/kubeconfig/ops.go`.

## Group-scoped alerts by tag

Instead of enabling alerts per-kubeconfig, apply them to every kubeconfig carrying a given tag (e.g. all `prod`-tagged). This would mean a `tag_policies:` section in the state file keyed by tag. Bigger design lift than the other roadmap items; revisit after cloud sync lands.
