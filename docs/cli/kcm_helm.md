## kcm helm

Run helm through the values-path / context mismatch guard

### Synopsis

Invokes helm with the given arguments. When the active kubeconfig has the helm-guard enabled, kcm parses -f/--values flags, derives a cluster/env name from each values-file path, and compares it to the active kubectl context. On a significant mismatch (e.g. path says k8s-test-01, context is k8s-prod-01), kcm prompts for confirmation before exec'ing helm.

```
kcm helm [args...] [flags]
```

### Options

```
  -h, --help   help for helm
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

