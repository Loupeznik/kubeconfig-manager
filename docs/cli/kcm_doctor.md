## kcm doctor

Run diagnostic checks against the local kcm + kubectl setup

### Synopsis

Walks through the common misconfigurations in the order they bite users: kubectl/helm on PATH, shell hook installed, state file schema, active kubeconfig resolved, palette populated, stale state entries. Exits non-zero if any check fails (warnings alone do not fail the exit code).

```
kcm doctor [flags]
```

### Options

```
  -h, --help   help for doctor
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

