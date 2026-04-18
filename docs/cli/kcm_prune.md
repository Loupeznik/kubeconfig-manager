## kcm prune

List (and optionally remove) state entries whose kubeconfig is gone or whose topology changed

### Synopsis

Walks the state file and reports entries that no longer match an actual
kubeconfig on disk — either the file referenced by path_hint is missing,
or its current stable fingerprint differs from the entry's key (meaning
the kubeconfig was edited in a way that changed its logical topology).

Default is a dry run: prints the list and exits. Pass --yes to actually
remove the stale entries.

```
kcm prune [flags]
```

### Options

```
      --dir string   Kubeconfig directory to match path_hints against (default: ~/.kube)
  -h, --help         help for prune
      --yes          Actually remove the stale entries (default: dry-run)
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

