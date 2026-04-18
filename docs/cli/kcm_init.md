## kcm init

First-run walkthrough: palette seed, shell hook, starship snippet

### Synopsis

Interactive setup that asks a handful of questions and performs the common first-time steps: adding a starter tag palette, installing the shell hook (with optional kubectl/helm aliases), and printing a starship custom-module snippet. Pass --yes to skip prompts and accept every default (useful for automation and for testing).

```
kcm init [flags]
```

### Options

```
  -h, --help              help for init
      --rc string         Override rc file path for the shell hook install (default: detected per shell)
      --skip-palette      Do not seed the tag palette
      --skip-shell-hook   Do not install the shell hook
      --yes               Skip interactive prompts and accept defaults
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

