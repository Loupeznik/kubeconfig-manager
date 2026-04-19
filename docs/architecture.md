# Architecture

## Package layout

```
cmd/kubeconfig-manager/    main entrypoint, wraps Cobra with charmbracelet/fang
internal/cli/              Cobra subcommand definitions (split per feature area)
internal/kubeconfig/       file discovery, clientcmd wrapper, stable + content hashing, merge/split/extract/remove
internal/state/            YAML state schema, Store interface, FileStore (flock + atomic write)
internal/guard/            verb extraction, kubectl + helm policy evaluation, huh confirmation, exec
internal/audit/            append-only audit log for guard decisions
internal/shell/            shell detection, export-line formatter, rc-hook installer
internal/tui/              Bubble Tea model split per mode: list / detail / palette / tag editor / rename / ctx ops / import+merge
scripts/                   build, lint, gendocs (go:build ignore)
docs/                      markdown docs (this directory) + generated cli/ and man/
.sisyphus/                 drafts for long-form reviews; not shipped
```

Only `cmd/` is importable from outside. Everything under `internal/` is unexported by Go convention.

## Dependency graph (direct)

```
cmd/kubeconfig-manager -> charmbracelet/fang, internal/cli
internal/cli           -> spf13/cobra, clientcmd, internal/{kubeconfig,state,shell,guard,audit,tui}
internal/kubeconfig    -> k8s.io/client-go/tools/clientcmd/api
internal/state         -> gopkg.in/yaml.v3, github.com/gofrs/flock, github.com/adrg/xdg
internal/guard         -> charmbracelet/huh, golang.org/x/term, internal/{kubeconfig,state}
internal/audit         -> github.com/adrg/xdg (stdlib otherwise)
internal/shell         -> (stdlib only)
internal/tui           -> charmbracelet/bubbletea + bubbles + lipgloss, internal/{kubeconfig,state}
```

No cycles. `internal/shell` and `internal/audit` are the lightest-weight packages; everything else depends on `kubeconfig` and `state` as the data layer.

## Data model at a glance

State lives under `$XDG_CONFIG_HOME/kubeconfig-manager/config.yaml` and holds:

- `available_tags` — the global tag palette (allow-list for `tag add` unless `--allow-new`).
- `helm_guard` — global helm-guard policy: enabled flag, pattern list, global fallback, env tokens.
- `entries[stable_hash]` — per-kubeconfig metadata: path_hint, file-level + per-context tags and alerts, and an optional per-entry `helm_guard` override.

See [state-file.md](state-file.md) for the full schema including the helm-guard fields.

### Stable vs content hashing

`internal/kubeconfig.IdentifyFile` returns both a **stable hash** (SHA-256 of the logical topology — cluster servers, context/user/cluster references) and a **content hash** (SHA-256 of the raw file). State entries are keyed by stable hash so metadata survives:

- `kubectl config use-context` flipping `current-context`.
- `kubens`/`kubectx`-style namespace switches.
- Re-serialization that rewrites keys or whitespace.

The content-hash key was the v0.8.x default; `state.Config.TakeEntry` falls back to it for a read and rekeys on first mutation, so upgrades migrate transparently (the pattern `oldID.ContentHash` is passed alongside `oldID.StableHash` in every call site).

### Guard policy resolution order

**kubectl (destructive-action guard, `internal/guard`):**
1. Per-context override, if one exists for the active context (`--context` flag or `current-context`).
2. File-level `Alerts` policy on the entry.
3. Built-in defaults from `state.DefaultBlockedVerbs()` when `Alerts.Enabled=true` but verbs unset.

An `Alerts{Enabled:false}` in `ContextAlerts[name]` with any other field set is treated as an explicit override suppressing the file-level policy.

**helm (values-path guard, `internal/guard/helm.go`):**
1. Per-entry `HelmGuard` override. A nil pointer on the entry means inherit; a struct with `Enabled` pointing at `false` explicitly disables.
2. Global `helm_guard` root block.
3. Falls back to `DefaultHelmPattern` ("clusters/{name}/") + `DefaultEnvTokens()`, with the guard **enabled by default** when neither side set `Enabled`. The field is `*bool` so "never configured" is distinguishable from "explicitly off"; `HelmGuard.IsEnabled()` resolves the effective boolean.

Pattern matching tries each pattern in `Patterns` in order; the first match wins. When none match and `GlobalFallback=true`, the raw path is tokenized and compared directly.

## Test layers

- **Unit tests** per package: hashing, merge/extract, state serialization, guard evaluation, exec stubs.
- **testscript suite** at `internal/cli/testdata/script/*.txt` drives the full CLI against a throwaway `$XDG_CONFIG_HOME`. In-process (no real binary build), so scripts run in sub-second time.
- **Benchmarks** at `internal/cli/starship_bench_test.go` gate the prompt-hot-path budget for `kcm starship`.
- TUI has no tests today — the prescribed tool is `charmbracelet/x/teatest` when that changes.

## Design decisions

### Why stable-hash keys for state

See [state-file.md](state-file.md#why-content-hash-keys). Short version: lets metadata survive renames and context flips, and lets future cloud sync unify entries across machines.

### Why clientcmd instead of a hand-rolled YAML parser

Kubeconfig has a non-trivial schema with moving parts (exec plugins, OIDC, various auth info types) and relative path resolution for certs. `k8s.io/client-go/tools/clientcmd` is the canonical implementation — using it means we can never drift from upstream kubectl's understanding of what a valid kubeconfig is. The transitive dependency weight (~50 MB of apimachinery) is a reasonable trade for correctness.

### Why opt-in `kubectl` / `helm` aliases, not transparent interception

Transparently hijacking `kubectl` or `helm` would require either a PATH-shadow binary or an eBPF-style hook — both fragile and surprising. The shell-alias approach is explicit, reversible with one command, and makes the user's consent clear.

### Why single-file state, not per-kubeconfig sidecar files

Sidecar files (`prod.yaml.kcm.yaml`) clutter the kubeconfig directory and break if you move the kubeconfig. A single, centrally-located state file with stable-hash keying is portable and harder to leave in a stale state.

### Why a one-line audit log instead of structured JSON

`kcm audit` writes one line per guard prompt in `key=value` form so grep, awk, and human eyes all work without a JSON parser. The values that can contain spaces are single-quoted. When the log grows beyond what `grep` is comfortable with, the user can pipe to `tail -n N`, and the `kcm audit --tail N` flag reads the last N lines natively.

### Why Bubble Tea for the TUI, not Go's built-in terminal packages

Bubble Tea provides the Elm-style update loop, which keeps state management predictable across modes (list / detail / palette / tag-edit / rename / context ops / import+merge). Each mode owns one `tui/*.go` file with its update and view functions; the dispatcher lives in `tui.go`.

### Why `fang` around `cobra`

`charmbracelet/fang` adds styled help pages, error rendering, automatic version handling, and man-page generation without touching the Cobra command tree. Zero cost to integrate (`fang.Execute(ctx, rootCmd)`), and it matches the visual style of the TUI.

## Extension points

If you're adding a new subcommand:

1. Create the Cobra command constructor in `internal/cli/` (one file per logical group).
2. Register it in `internal/cli/root.go`'s `NewRootCmd`.
3. Run `go run scripts/gendocs.go` to regenerate the markdown and man pages.
4. If it mutates state, go through `state.Store.Mutate()` to get flock-serialized safety.
5. If it needs to resolve a kubeconfig by name, use `kubeconfig.ResolvePath()` + `kubeconfig.IdentifyFile()` to get both hashes in one call.

For a new Store backend (cloud sync), implement the `state.Store` interface and wire it behind a `--state-backend` flag or env var. Nothing outside `internal/state` needs to change.

If you're adding a new TUI mode, drop a `tui/<mode>_view.go` file with `update<Mode>` and `view<Mode>` methods on Model, then add one case each to the dispatcher in `tui/tui.go`.
