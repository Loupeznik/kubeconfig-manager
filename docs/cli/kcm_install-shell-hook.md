## kcm install-shell-hook

Install shell integration (kcm function and kubectl alias)

### Synopsis

Installs a fenced block in your shell rc with two things:
  1. A kcm() function that evaluates the export snippet from `kcm use` and `kcm tui`.
  2. An alias so plain `kubectl` routes through kcm's destructive-action guard.

Pass --no-alias-kubectl to skip the kubectl alias (alerts will only fire when you
run `kcm kubectl ...` explicitly).

```
kcm install-shell-hook [flags]
```

### Options

```
      --alias-helm         Also alias helm to route through the helm-guard (opt-in)
  -h, --help               help for install-shell-hook
      --no-alias-kubectl   Skip the kubectl alias (alerts won't fire for plain kubectl invocations)
      --rc string          rc file path (default depends on shell)
      --shell string       bash, zsh, or pwsh (auto-detected if unset)
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

