# Helm values-path guard

The classic foot-gun when working with multiple Kubernetes clusters: your active `kubectl` context is `k8s-prod-01`, and you type

```sh
helm upgrade myapp -f deploy/clusters/k8s-test-01/values.yaml
```

The helm command has no idea your context is prod — it just applies the test values and waits for the fallout. `kcm helm` catches this before it happens.

## How it works

When the helm guard is enabled, `kcm helm <args>` does three things before invoking helm:

1. Parses `-f` / `--values` flags from `args` (including the `--values=path` and `-f a,b,c` comma-list forms).
2. Derives a cluster/env name from each values-file path using a configurable pattern (default `clusters/{name}/`). For `deploy/clusters/k8s-test-01/values.yaml` this yields `k8s-test-01`.
3. Compares each derived name to the active kubectl context (and its cluster) using a token-based fuzzy match. If they contradict, `kcm` prompts for confirmation.

The match has three outcomes:

| Severity | When it fires | What to do |
|---|---|---|
| **OK** | Derived name shares a token with the context (env or otherwise). | No alert. |
| **Soft** | No token overlap at all — possible user error, but no smoking gun. | Prompt. |
| **Hard** | Both sides contain environment tokens that contradict each other (e.g. path has `test`, context has `prod`). | Prompt — this is the dangerous case. |

"Environment tokens" default to `prod production staging stg stage dev development test tst qa sandbox sbx preprod preview`. You can override the list per-entry or globally.

## Enabling the guard

The guard is **off by default.** Turn it on globally or per-kubeconfig:

```sh
# Globally — applies to every kubeconfig unless overridden
kubeconfig-manager helm-guard enable

# Per-kubeconfig — overrides the global setting for this one file
kubeconfig-manager helm-guard enable --file ~/.kube/prod.yaml

# Explicitly disable for one kubeconfig while global stays on
kubeconfig-manager helm-guard disable --file ~/.kube/sandbox.yaml
```

Resolution order: per-entry override > global > off (default).

View the effective policy:

```sh
kubeconfig-manager helm-guard show                            # global
kubeconfig-manager helm-guard show --file ~/.kube/prod.yaml   # per-file with inheritance breakdown
```

## Customizing the path pattern

Not every repo lays values files out as `clusters/<name>/`. Change the pattern globally or per-file — the `{name}` placeholder marks the capture group:

```sh
kubeconfig-manager helm-guard set-pattern "environments/{name}/"
kubeconfig-manager helm-guard set-pattern "envs/{name}.yaml" --file prod
```

The match is path-position-insensitive: `deploy/environments/prod-eu/values.yaml` and `environments/prod-eu/x.yaml` both match `environments/{name}/`.

If no `-f` path matches the pattern, no name is derived and no alert fires for that invocation — the guard stays silent on paths it doesn't understand.

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
  pattern: "clusters/{name}/"
  env_tokens: [prod, production, staging, dev, test, qa]

entries:
  sha256:...:
    helm_guard:
      enabled: false   # explicit per-entry disable; overrides global
```

Per-entry fields left empty fall back to global, which in turn falls back to built-in defaults. See [`docs/state-file.md`](state-file.md) for the full schema.
