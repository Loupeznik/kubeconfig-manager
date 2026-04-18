## kcm audit

Show the guard-prompt audit log (kubectl + helm approvals / aborts)

### Synopsis

Prints the last --tail entries from the audit log written by the kubectl and helm guards on every prompt. The log lives under XDG_DATA_HOME/kubeconfig-manager/audit.log and uses a one-line key=value format so grep/awk work naturally.

```
kcm audit [flags]
```

### Options

```
  -h, --help       help for audit
      --tail int   Number of most-recent entries to show (0 = all) (default 20)
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

