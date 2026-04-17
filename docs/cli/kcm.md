## kcm

Manage kubeconfig files and kubectl contexts

### Synopsis

kubeconfig-manager (kcm) is a TUI + CLI for managing local kubeconfig files and kubectl contexts, with tagging and destructive-action guardrails.

### Options

```
  -h, --help   help for kcm
```

### SEE ALSO

* [kcm alert](kcm_alert.md)	 - Configure destructive-action alerts per kubeconfig or context
* [kcm contexts](kcm_contexts.md)	 - List contexts in the default kubeconfig (~/.kube/config)
* [kcm import](kcm_import.md)	 - Merge a kubeconfig file into the default ~/.kube/config (or --into)
* [kcm install-shell-hook](kcm_install-shell-hook.md)	 - Install shell integration (kcm function, optional kubectl alias)
* [kcm kubectl](kcm_kubectl.md)	 - Run kubectl through the destructive-action guard
* [kcm list](kcm_list.md)	 - List kubeconfig files in the managed directory
* [kcm merge](kcm_merge.md)	 - Merge two kubeconfig files into a new file
* [kcm rename](kcm_rename.md)	 - Rename a kubeconfig file on disk (metadata re-binds automatically)
* [kcm show](kcm_show.md)	 - Show contexts, clusters, and users of a kubeconfig file
* [kcm split](kcm_split.md)	 - Extract a context (with its cluster and user) into its own file
* [kcm tag](kcm_tag.md)	 - Manage tags on kubeconfig files
* [kcm tui](kcm_tui.md)	 - Launch the interactive TUI
* [kcm uninstall-shell-hook](kcm_uninstall-shell-hook.md)	 - Remove the shell integration block from the rc file
* [kcm use](kcm_use.md)	 - Print a shell snippet that exports KUBECONFIG to the selected file

