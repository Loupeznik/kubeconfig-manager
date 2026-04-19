# Demo walkthrough

Step-by-step commands for the README showcase cast. Run `./scripts/demo/setup.sh` first, then `. .temp/demo/env.sh`, then start the recording:

```sh
asciinema rec docs/images/demo.cast \
  --title 'kubeconfig-manager — prod/test guard in action' \
  --command zsh \
  --idle-time-limit 2 \
  --overwrite
```

Inside the recording you'll be in a clean zsh with the sandbox `.zshrc` loaded: starship + `kcm` custom module on the prompt, and the kcm shell hook installed so `kubectl` / `helm` / `kcm use` all just work.

Timing tip: type at a natural pace. `--idle-time-limit 2` trims long pauses automatically. Aim for a total of ~45–60 seconds.

## Beat 1 — the pitch (≈10 s)

```sh
clear
kcm list
```

Two tidy dummy kubeconfigs with tags and the `⚠ ALERT` badge on prod. The starship prompt shows `⎈ prod,eu,critical ⚠` because the current `$KUBECONFIG` is `prod.yaml` — the tool advertises itself right at the prompt.

## Beat 2 — switch kubeconfigs (≈10 s)

```sh
kcm use test
```

Because the shell hook is installed, `kcm use` evals its export snippet in place — `$KUBECONFIG` flips to `test.yaml` and the starship module immediately updates to `⎈ staging`. Run `echo $KUBECONFIG` if you want viewers to see the variable explicitly.

```sh
kcm use prod
```

Flip back. The `⚠` badge reappears.

## Beat 3 — pure CLI op: split a context (≈10 s)

```sh
kcm split k8s-prod-02 ~/.kube/prod-02.yaml --from ~/.kube/prod.yaml
kcm list
```

`kcm split` extracts the `k8s-prod-02` context into its own file, leaving the source intact. The subsequent `kcm list` shows a new `prod-02.yaml` row. This is the "it's a real kubeconfig manipulator, not just a browser" beat.

## Beat 4 — the guard saves you (≈15 s)

```sh
# Active context is k8s-prod-01. Aim at a test values file by mistake:
helm upgrade myapp -f work/helm-values/clusters/k8s-test-01/values.yaml
```

Plain `helm` here — the shell hook routes it through the guard, which detects the prod/test mismatch, prints the comparison, and prompts. **Pick "No, abort"**. The starship badge showing `⚠ prod,eu,critical` on the prompt is extra visual proof of what context we're on.

## Beat 5 — silent on the matching path (≈5 s)

```sh
helm upgrade myapp -f work/helm-values/clusters/k8s-prod-01/values.yaml
```

Path matches context — no prompt, the helm stub fires.

## Beat 6 — TUI tour (≈20 s)

```sh
kcm tui
```

1. Down/up through the kubeconfigs — the list shows tags + alert badges update.
2. `enter` on `prod.yaml` — detail view shows `k8s-prod-01` (current) + `k8s-prod-02`.
3. `t` on `k8s-prod-02` — tag picker opens with `prod`, `eu`, `critical` already marked (inherited from file level). `space` on `eu` to deselect — this creates a per-context exclusion so `eu` no longer applies to `k8s-prod-02` only. `enter` to save.
4. `R` on `k8s-prod-02` — rename context to `k8s-prod-eu`. Tags + exclusions + alerts follow the rename.
5. `esc` back to file list, `p` for the palette, `esc` out.
6. `s` on `prod.yaml` — exits the TUI and evals the export line in your shell; the prompt updates.

## Beat 7 — close (≈5 s)

```sh
kcm audit --tail 3
```

Three lines — `decision=declined` for the aborted helm, `decision=approved` for the one that went through, and whatever you touched in the TUI if it hit a guard. Reinforces "it remembers what you did".

End recording with `ctrl+d` inside the recording zsh (or `ctrl+c` on `asciinema rec`).
