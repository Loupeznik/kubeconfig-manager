## kcm helm-guard set-patterns

Replace the path-pattern list used to derive cluster/env names

### Synopsis

Replaces the entire pattern list with the given patterns. Each pattern must contain the {name} placeholder (capture stops at the next slash). Patterns are tried in order and the first match wins. See also add-pattern, remove-pattern, fallback.

```
kcm helm-guard set-patterns <pattern...> [flags]
```

### Options

```
      --dir string    Kubeconfig directory (default: ~/.kube)
      --file string   Apply only to this kubeconfig (default: global)
  -h, --help          help for set-patterns
```

### SEE ALSO

* [kcm helm-guard](kcm_helm-guard.md)	 - Configure the helm values-path / context mismatch guard

