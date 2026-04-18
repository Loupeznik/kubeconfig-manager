## kcm starship

Print a one-line tag/alert summary for starship's custom module

### Synopsis

Prints a minimal summary of the active kubeconfig's tags and alert state,
suitable for consumption by starship's custom module. Exits silently with no
output when there is nothing worth showing — starship's 'when' predicate hides
the module in that case.

Output format:
  "⚠ prod,eu,critical"    — alerts enabled + tags
  "prod,eu"               — tags only
  "⚠"                     — alerts only
  ""                      — neither (module suppressed by starship)

Recommended starship config:

  [custom.kcm]
  command = "kubeconfig-manager starship"
  when = "kubeconfig-manager starship | grep -q ."
  format = "[$output]($style) "
  style = "bold yellow"

Then reference it in your top-level format, e.g.:
  format = "...${custom.kcm}..."

Do NOT set shell = ["sh", "-c"] — starship 1.24 mishandles the combination
of an explicit shell override and the custom command, and the module emits
nothing.

```
kcm starship [flags]
```

### Options

```
      --context string   Context to summarize (default: the kubeconfig's current-context)
      --file string      Kubeconfig path (default: first of $KUBECONFIG, else ~/.kube/config)
  -h, --help             help for starship
```

### SEE ALSO

* [kcm](kcm.md)	 - Manage kubeconfig files and kubectl contexts

