# Destructive-action guard

`kcm kubectl <args>` wraps the real `kubectl`. When the active kubeconfig has alerts enabled and the verb you're running is in the blocked list, `kcm` prompts for confirmation before exec'ing kubectl.

## Activation scopes

By default, only `kcm kubectl` invocations go through the guard. To intercept every `kubectl` call — including direct ones and ones made by scripts — add the opt-in alias:

```sh
kubeconfig-manager install-shell-hook --alias-kubectl
```

See [shell-integration.md](shell-integration.md#opt-in-route-kubectl-through-the-guard) for the trade-offs.

## How a decision is made

1. Parse the args to find the verb (first non-flag positional argument, skipping known global flags with values like `--context foo`, `-n ns`, `--kubeconfig path`).
2. Split `$KUBECONFIG` on the OS path separator (`:` on unix, `;` on Windows). If unset, use `~/.kube/config`.
3. For each path, compute `sha256:<hex>` and look it up in the state file.
4. If the entry has `alerts.enabled: true` and the verb appears in the entry's `blocked_verbs` (or the defaults if that list is empty), the decision triggers.

## Confirmation flow

When a trigger fires, `kcm` prints a summary to stderr and prompts via [huh](https://github.com/charmbracelet/huh):

```
Destructive verb "delete" will run against:
  - /Users/you/.kube/prod.yaml (context "prod-eu", cluster "prod-eu-cluster", tags: prod,eu,critical)

Proceed with kubectl delete?
> Yes, proceed
  No, abort
```

If `confirm_cluster_name: true` is set in the alert policy, you're asked to **type** the cluster name rather than picking yes/no — a high-friction safety net for critical clusters.

## No-TTY behavior

If stdin or stderr isn't a TTY (e.g. CI, `cron`, piped input), `kcm` refuses to proceed with an error:

```
no TTY available for confirmation prompt (alerts enabled; run in an interactive shell or disable alerts)
```

This is intentional — silently approving destructive commands in non-interactive contexts is a footgun.

## Exec semantics

On approval, `kcm` runs `kubectl <args>` with your inherited stdin/stdout/stderr and the same environment. The child's exit code is propagated to the caller.

## Managing which verbs trigger

The default list (`delete drain cordon uncordon taint replace patch`) covers the most common destructive or state-modifying verbs. `apply` is intentionally **not** in the default list — it's used for routine deploys. If you want a stricter policy for `prod`, edit the state file directly to add `apply`.
