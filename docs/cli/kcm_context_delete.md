## kcm context delete

Delete a context (and its per-context tags/alerts) from a kubeconfig

```
kcm context delete <file> <name> [flags]
```

### Options

```
      --dir string     Kubeconfig directory (default: ~/.kube)
  -h, --help           help for delete
      --keep-orphans   Keep the referenced cluster/user even if no other context uses them
```

### SEE ALSO

* [kcm context](kcm_context.md)	 - Rename or delete contexts within a kubeconfig file

