# Helm values-path guard

The classic foot-gun when working with multiple Kubernetes clusters: your active `kubectl` context is `k8s-prod-01`, and you type

```sh
helm upgrade myapp -f deploy/clusters/k8s-test-01/values.yaml
```

The helm command has no idea your context is prod — it just applies the test values and waits for the fallout. `kcm helm` catches this before it happens.

## How it works

When the helm guard is enabled, `kcm helm <args>` does three things before invoking helm:

1. Parses `-f` / `--values` flags from `args` (including the `--values=path` and `-f a,b,c` comma-list forms).
2. Derives a cluster/env name from each values-file path by trying the configured patterns in order — the first match wins. Default pattern is `clusters/{name}/`, so `deploy/clusters/k8s-test-01/values.yaml` yields `k8s-test-01`. If no pattern matches and the *global fallback* is on, the raw path itself is tokenized instead.
3. Compares each derived name (or raw path tokens) to the active kubectl context (and its cluster) using a token-based fuzzy match. If they contradict, `kcm` prompts for confirmation.

The match has three outcomes:

| Severity | When it fires | What to do |
|---|---|---|
| **OK** | Derived name shares a token with the context (env or otherwise). | No alert. |
| **Soft** | No token overlap at all — possible user error, but no smoking gun. | Prompt. |
| **Hard** | Both sides contain environment tokens that contradict each other (e.g. path has `test`, context has `prod`). | Prompt — this is the dangerous case. |

"Environment tokens" default to `prod production staging stg stage dev development test tst qa sandbox sbx preprod preview`. You can override the list per-entry or globally.

## Enabling the guard

The guard is **on by default** — as a safety feature, it protects every kubeconfig without any configuration. You can still turn it off explicitly, globally or per-kubeconfig:

```sh
# Opt out globally (you almost certainly don't want this)
kubeconfig-manager helm-guard disable

# Opt out for one specific kubeconfig (e.g. a throwaway sandbox)
kubeconfig-manager helm-guard disable --file ~/.kube/sandbox.yaml

# Re-enable after a previous disable (explicit form)
kubeconfig-manager helm-guard enable --file ~/.kube/prod.yaml
```

Resolution order: per-entry override > global > default (ON). A per-entry `enabled: false` is an explicit suppression and wins over the global default.

View the effective policy:

```sh
kubeconfig-manager helm-guard show                            # global
kubeconfig-manager helm-guard show --file ~/.kube/prod.yaml   # per-file with inheritance breakdown
```

## Customizing path patterns

Not every repo lays values files out as `clusters/<name>/`. You can configure a list of patterns — they're tried in order, first match wins. The `{name}` placeholder is the capture group (it stops at the next `/`).

Replace the whole list:

```sh
kubeconfig-manager helm-guard set-patterns "environments/{name}/" "clusters/{name}/"
kubeconfig-manager helm-guard set-patterns "envs/{name}.yaml" --file prod
```

Append or drop individual patterns:

```sh
kubeconfig-manager helm-guard add-pattern "deploy/{name}/values.yaml"
kubeconfig-manager helm-guard remove-pattern "clusters/{name}/"
```

The match is path-position-insensitive: `deploy/environments/prod-eu/values.yaml` and `environments/prod-eu/x.yaml` both match `environments/{name}/`.

### Global fallback (no pattern, match on path tokens)

If your values files don't follow any directory convention (e.g. `helm/my-app.prod.yaml`), flip on the global fallback. When no pattern matches, the guard tokenizes the raw path — splitting on `/`, `-`, `_`, `.` — and compares those tokens directly against the active context/cluster and the environment-token list:

```sh
kubeconfig-manager helm-guard fallback on
kubeconfig-manager helm-guard fallback off
kubeconfig-manager helm-guard fallback on --file ~/.kube/prod.yaml
```

So `helm/my-app.test.yaml` with an active `prod-eu` context will still trigger a hard mismatch (path contains `test`, context contains `prod`).

If both pattern matching and fallback fail to derive/overlap anything meaningful, no alert fires for that invocation — the guard stays silent on paths it doesn't understand.

## Routing plain `helm` through the guard

By default only `kcm helm` invocations trigger the check. To intercept every `helm` call (direct, aliased, or scripted), opt in when installing the shell hook:

```sh
kubeconfig-manager install-shell-hook --alias-helm
```

This adds `alias helm='command kubeconfig-manager helm'` inside the fenced block. Trade-offs mirror the `--alias-kubectl` flag: one extra process per invocation (~1ms), easy to uninstall by re-running without the flag.

## Example session

```sh
# Setup
kubeconfig-manager helm-guard enable
kubeconfig-manager install-shell-hook --alias-helm
exec zsh

# Active context is k8s-prod-01. Accidentally run a test-cluster upgrade:
helm upgrade myapp -f deploy/clusters/k8s-test-01/values.yaml

# kcm intercepts:
#   helm values-path / context mismatch detected:
#     active context: k8s-prod-01 (cluster k8s-prod-01)
#     - deploy/clusters/k8s-test-01/values.yaml
#         derived name: "k8s-test-01" — severity: hard (path environment [test] does not match context/cluster environment [prod])
#
#   Proceed with this helm invocation?
#   > Yes, proceed
#     No, abort
```

Pick No, context-switch to the intended cluster, run the command again. The context-matching values file triggers nothing and helm runs unchanged.

## No-TTY behavior

If stdin or stderr isn't a TTY (CI, cron, piped input), `kcm helm` refuses to proceed when the guard triggers, with:

```
no TTY available for confirmation prompt (helm-guard triggered; run in an interactive shell or disable the guard)
```

Matching the `kcm kubectl` policy — silently approving destructive helm commands in non-interactive contexts would defeat the purpose.

## State schema

Per kubeconfig (under `entries[hash].helm_guard`) and globally (at state-file root):

```yaml
helm_guard:
  enabled: true
  patterns:
    - "clusters/{name}/"
    - "envs/{name}.yaml"
  global_fallback: true
  env_tokens: [prod, production, staging, dev, test, qa]

entries:
  sha256:...:
    helm_guard:
      enabled: false   # explicit per-entry disable; overrides global
```

State files written by v0.10.x used a single `pattern:` scalar field instead of `patterns:`. That form still loads cleanly — on first write, it's migrated to the list form.

Per-entry fields left empty fall back to global, which in turn falls back to built-in defaults. See [`docs/state-file.md`](state-file.md) for the full schema.
