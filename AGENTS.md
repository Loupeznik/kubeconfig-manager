# Agent guide

Project-specific guidance for LLM agents working in this repo. Read this before touching code.

## Repo at a glance

- Go 1.26 CLI + TUI. Module path: `github.com/loupeznik/kubeconfig-manager`.
- Binary name: `kcm` (also `kubeconfig-manager`). Entry point: `cmd/kubeconfig-manager`.
- Dependencies: Cobra (+ charmbracelet/fang), charmbracelet/bubbletea, charmbracelet/huh, k8s.io/client-go.
- Persistence: local YAML state under `$XDG_CONFIG_HOME/kubeconfig-manager/config.yaml`, atomic writes + flock. See `docs/state-file.md`.

## Before committing

A PreToolUse hook at `.claude/hooks/pre-commit-check.sh` runs these on every `git commit` call and blocks if any fail:

1. `gofmt -l .` — every Go file must be formatted.
2. `go vet ./...` — no vet issues.
3. `go test ./...` — all tests pass.
4. `golangci-lint run ./...` — skipped silently if not installed locally (CI enforces regardless).
5. `goreleaser check` — skipped silently if not installed locally (CI enforces regardless).

To run the checks manually:

```sh
gofmt -l .
go vet ./...
go test ./...
golangci-lint run ./...
goreleaser check
# before pushing, also:
go test -race ./...
```

Do not use `--no-verify` or equivalent to bypass the hook; fix the underlying issue instead.

## Commit messages

- Present-tense imperative mood, single line, ≤72 chars.
- No trailing `Co-Authored-By:` lines — this project does not use them.
- No `[skip ci]` unless the commit is genuinely docs/config only and the user explicitly asks.

## Where to put new code

| You want to add…                       | Put it in…                                        |
| -------------------------------------- | ------------------------------------------------- |
| A new `kcm` subcommand                 | One `internal/cli/<feature>.go` file              |
| Register the subcommand                | `internal/cli/root.go` — `NewRootCmd()` AddCommand|
| A new TUI mode                         | `internal/tui/<mode>_view.go` + dispatcher wire   |
| A change to the persisted schema       | `internal/state/schema.go` (bump `CurrentVersion` if breaking) |
| A state mutation                       | Go through `state.Store.Mutate()` for flock safety |
| A helper that resolves paths by name   | `internal/kubeconfig/kubeconfig.go`               |
| A new shell integration                | `internal/shell/`                                 |
| A new guard check                      | `internal/guard/` — extend `Evaluate` or `EvaluateHelm` |

## Regenerate CLI docs after Cobra changes

```sh
go run scripts/gendocs.go
```

Commit the regenerated `docs/cli/*.md` and `docs/man/*.1` in the same commit as the Cobra change.

## Testing conventions

- **Unit tests:** one `*_test.go` per package, standard `testing` package, no external test runner.
- **End-to-end:** `internal/cli/testdata/script/*.txt` driven by `go-internal/testscript`. Fast (in-process) — prefer these over shell scripts for multi-step CLI scenarios.
- **TUI:** no tests; when/if that changes, use `charmbracelet/x/teatest`.
- **State isolation:** tests must set `XDG_CONFIG_HOME` to a `t.TempDir()` and call `xdg.Reload()`. The `isolateState(testing.TB, stateHome)` helper in `internal/cli/cli_test.go` does this.
- **Manual smoke-testing:** use `./.temp/` as a sandbox. Never touch the real `~/.kube` or the user's real `$XDG_CONFIG_HOME`.

## Cobra flag style

- `--dry-run` on every mutating command that touches disk or state. Prints `[dry-run] would ...` and returns cleanly.
- `--dir` defaults to `~/.kube` via `resolveDir("")`. Always plumb through this helper.
- `--context`, `--file` are the standard per-context / per-file filter flags.
- Tab completion: wire via `cmd.ValidArgsFunction = completeKubeconfigNames` (or friends in `internal/cli/completion.go`).

## Guard + audit

- The kubectl and helm guards both go through `internal/guard`. Prompt UX via `charmbracelet/huh`; no-TTY fails closed.
- Every prompt (approved, declined, no-tty) appends one line to `$XDG_DATA_HOME/kubeconfig-manager/audit.log` via `internal/audit`. Call `audit.Append` from the CLI wrapper, not from the guard package.

## Helm-guard defaults

- `HelmGuard.Enabled` is `*bool` (tri-state): nil → default ON, explicit true/false override. Always read via `HelmGuard.IsEnabled()`.
- Default pattern: `clusters/{name}/`. Default env tokens: `state.DefaultEnvTokens()`.
- `GlobalFallback` is plain bool, default false. Opt-in because raw-path tokenization is noisy.
- Legacy single `pattern:` YAML field is auto-migrated to `patterns:` list on load (`UnmarshalYAML` in `schema.go`).

## Release + Docker

- `.goreleaser.yaml` uses `dockers_v2:` (requires goreleaser ≥ 2.14). Pinned to `~> v2.14` in both workflows.
- `Dockerfile.goreleaser` is the v2-compatible dockerfile; the top-level `Dockerfile` is for local `docker build .` dev use.
- Multi-arch image published to `ghcr.io/loupeznik/kubeconfig-manager:{version,latest}` on tag push (v*). Prereleases do not get the `latest` tag.

## Things to avoid

- Don't silently rewrite the kubeconfig schema — `clientcmd.LoadFromFile` + `WriteToFile` preserves it; stick to that path.
- Don't add feature flags or back-compat shims — the state-file migration pattern is the correct tool when schemas change.
- Don't add trailing summaries at the end of your turns; the user reads the diff.
- Don't create new docs/README/guide files unless the user explicitly asks.
- Don't add comments that restate what the code does. `WHY` comments only.
