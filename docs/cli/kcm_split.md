## kcm split

Extract a context (with its cluster and user) into its own file

```
kcm split <context> <out-file> [flags]
```

### Options

```
      --dry-run       Print the planned change without writing
      --from string   Source kubeconfig (default: ~/.kube/config)
  -h, --help          help for split
      --remove        Remove the extracted context from the source file
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

