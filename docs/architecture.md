# Architecture

## Package layout

```
cmd/kubeconfig-manager/    main entrypoint, wraps Cobra with charmbracelet/fang
internal/cli/              Cobra subcommand definitions (root, commands, ops, state_commands)
internal/kubeconfig/       file discovery, clientcmd wrapper, hashing, merge/split/extract/remove
internal/state/            YAML state schema, Store interface, FileStore (flock + atomic write)
internal/guard/            verb extraction, policy evaluation, huh confirmation, kubectl exec
internal/shell/            shell detection, export-line formatter, rc-hook installer
internal/tui/              Bubble Tea model with list/detail/tag-edit/rename modes
scripts/                   build, lint, gendocs (go:build ignore)
docs/                      markdown docs (this directory) + generated cli/ and man/
```

Only `cmd/` is importable from outside. Everything under `internal/` is unexported by Go convention.

## Dependency graph (direct)

```
cmd/kubeconfig-manager -> charmbracelet/fang, internal/cli
internal/cli           -> spf13/cobra, clientcmd, internal/{kubeconfig,state,shell,guard,tui}
internal/kubeconfig    -> k8s.io/client-go/tools/clientcmd/api
internal/state         -> gopkg.in/yaml.v3, github.com/gofrs/flock, github.com/adrg/xdg
internal/guard         -> charmbracelet/huh, golang.org/x/term, internal/{kubeconfig,state}
internal/shell         -> (stdlib only)
internal/tui           -> charmbracelet/bubbletea + bubbles + lipgloss, internal/{kubeconfig,state}
```

No cycles. `internal/shell` is stdlib-only and the lightest-weight package; everything else depends on `kubeconfig` and `state` as the data layer.

## Design decisions

### Why content-hash keys for state

See [state-file.md](state-file.md#why-content-hash-keys). Short version: lets metadata survive renames and lets future cloud sync unify entries across machines.

### Why clientcmd instead of a hand-rolled YAML parser

Kubeconfig has a non-trivial schema with moving parts (exec plugins, OIDC, various auth info types) and relative path resolution for certs. `k8s.io/client-go/tools/clientcmd` is the canonical implementation — using it means we can never drift from upstream kubectl's understanding of what a valid kubeconfig is. The transitive dependency weight (~50 MB of apimachinery) is a reasonable trade for correctness.

### Why opt-in `kubectl` alias, not transparent interception

Transparently hijacking `kubectl` would require either a PATH-shadow binary or an eBPF-style hook — both fragile and surprising. The shell-alias approach is explicit, reversible with one command, and makes the user's consent clear.

### Why single-file state, not per-kubeconfig sidecar files

Sidecar files (`prod.yaml.kcm.yaml`) clutter the kubeconfig directory and break if you move the kubeconfig. A single, centrally-located state file with content-hash keying is portable and harder to leave in a stale state.

### Why Bubble Tea for the TUI, not Go's built-in terminal packages

Bubble Tea provides the Elm-style update loop, which keeps state management predictable across modes (list / detail / tag-edit / rename). The cost is a few MB of binary weight — acceptable for a CLI that's already pulling client-go.

### Why `fang` around `cobra`

`charmbracelet/fang` adds styled help pages, error rendering, automatic version handling, and man-page generation without touching the Cobra command tree. Zero cost to integrate (`fang.Execute(ctx, rootCmd)`), and it matches the visual style of the TUI.

## Extension points

If you're adding a new subcommand:

1. Create the Cobra command constructor in `internal/cli/` (one file per logical group).
2. Register it in `internal/cli/root.go`'s `NewRootCmd`.
3. Run `go run scripts/gendocs.go` to regenerate the markdown and man pages.
4. If it mutates state, go through `state.Store.Mutate()` to get flock-serialized safety.
5. If it needs to resolve a kubeconfig by name, use `kubeconfig.ResolvePath()` + `kubeconfig.HashFile()`.

For a new Store backend (cloud sync), implement the `state.Store` interface and wire it behind a `--state-backend` flag or env var. Nothing outside `internal/state` needs to change.
