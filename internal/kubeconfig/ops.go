package kubeconfig

import (
	"fmt"
	"sort"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ConflictPolicy int

const (
	ConflictError ConflictPolicy = iota
	ConflictSkip
	ConflictOverwrite
)

func ParseConflictPolicy(s string) (ConflictPolicy, error) {
	switch s {
	case "error", "":
		return ConflictError, nil
	case "skip":
		return ConflictSkip, nil
	case "overwrite":
		return ConflictOverwrite, nil
	}
	return ConflictError, fmt.Errorf("unknown conflict policy %q (valid: error, skip, overwrite)", s)
}

type Collisions struct {
	Clusters  []string
	AuthInfos []string
	Contexts  []string
}

func (c Collisions) IsEmpty() bool {
	return len(c.Clusters) == 0 && len(c.AuthInfos) == 0 && len(c.Contexts) == 0
}

func (c Collisions) Error() string {
	return fmt.Sprintf(
		"merge collisions: clusters=%v authinfos=%v contexts=%v (use --on-conflict=skip or --on-conflict=overwrite)",
		c.Clusters, c.AuthInfos, c.Contexts,
	)
}

func newConfig() *clientcmdapi.Config {
	return &clientcmdapi.Config{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters:   map[string]*clientcmdapi.Cluster{},
		AuthInfos:  map[string]*clientcmdapi.AuthInfo{},
		Contexts:   map[string]*clientcmdapi.Context{},
	}
}

func Merge(dest, src *clientcmdapi.Config, policy ConflictPolicy) (*clientcmdapi.Config, Collisions, error) {
	if dest == nil {
		dest = newConfig()
	}
	merged := cloneConfig(dest)

	var col Collisions
	for _, name := range sortedClusterKeys(src.Clusters) {
		if _, exists := merged.Clusters[name]; exists {
			switch policy {
			case ConflictError:
				col.Clusters = append(col.Clusters, name)
				continue
			case ConflictSkip:
				continue
			case ConflictOverwrite:
			}
		}
		merged.Clusters[name] = src.Clusters[name].DeepCopy()
	}
	for _, name := range sortedAuthInfoKeys(src.AuthInfos) {
		if _, exists := merged.AuthInfos[name]; exists {
			switch policy {
			case ConflictError:
				col.AuthInfos = append(col.AuthInfos, name)
				continue
			case ConflictSkip:
				continue
			case ConflictOverwrite:
			}
		}
		merged.AuthInfos[name] = src.AuthInfos[name].DeepCopy()
	}
	for _, name := range sortedContextKeys(src.Contexts) {
		if _, exists := merged.Contexts[name]; exists {
			switch policy {
			case ConflictError:
				col.Contexts = append(col.Contexts, name)
				continue
			case ConflictSkip:
				continue
			case ConflictOverwrite:
			}
		}
		merged.Contexts[name] = src.Contexts[name].DeepCopy()
	}

	if policy == ConflictError && !col.IsEmpty() {
		return nil, col, col
	}
	if merged.CurrentContext == "" {
		merged.CurrentContext = src.CurrentContext
	}
	return merged, Collisions{}, nil
}

func Extract(src *clientcmdapi.Config, contextName string) (*clientcmdapi.Config, error) {
	if src == nil {
		return nil, fmt.Errorf("nil source config")
	}
	ctx, ok := src.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("context %q not found in source", contextName)
	}

	out := newConfig()
	out.Contexts[contextName] = ctx.DeepCopy()
	out.CurrentContext = contextName

	if ctx.Cluster != "" {
		if cluster, ok := src.Clusters[ctx.Cluster]; ok {
			out.Clusters[ctx.Cluster] = cluster.DeepCopy()
		} else {
			return nil, fmt.Errorf("context %q references missing cluster %q", contextName, ctx.Cluster)
		}
	}
	if ctx.AuthInfo != "" {
		if auth, ok := src.AuthInfos[ctx.AuthInfo]; ok {
			out.AuthInfos[ctx.AuthInfo] = auth.DeepCopy()
		} else {
			return nil, fmt.Errorf("context %q references missing user %q", contextName, ctx.AuthInfo)
		}
	}
	return out, nil
}

func Remove(src *clientcmdapi.Config, contextName string) (*clientcmdapi.Config, error) {
	if src == nil {
		return nil, fmt.Errorf("nil source config")
	}
	ctx, ok := src.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("context %q not found", contextName)
	}
	out := cloneConfig(src)
	delete(out.Contexts, contextName)

	if ctx.Cluster != "" && !isClusterReferenced(out, ctx.Cluster) {
		delete(out.Clusters, ctx.Cluster)
	}
	if ctx.AuthInfo != "" && !isAuthInfoReferenced(out, ctx.AuthInfo) {
		delete(out.AuthInfos, ctx.AuthInfo)
	}
	if out.CurrentContext == contextName {
		out.CurrentContext = ""
	}
	return out, nil
}

func cloneConfig(c *clientcmdapi.Config) *clientcmdapi.Config {
	out := newConfig()
	out.APIVersion = c.APIVersion
	out.Kind = c.Kind
	out.CurrentContext = c.CurrentContext
	out.Preferences = *c.Preferences.DeepCopy()
	for k, v := range c.Clusters {
		out.Clusters[k] = v.DeepCopy()
	}
	for k, v := range c.AuthInfos {
		out.AuthInfos[k] = v.DeepCopy()
	}
	for k, v := range c.Contexts {
		out.Contexts[k] = v.DeepCopy()
	}
	return out
}

func isClusterReferenced(c *clientcmdapi.Config, name string) bool {
	for _, ctx := range c.Contexts {
		if ctx != nil && ctx.Cluster == name {
			return true
		}
	}
	return false
}

func isAuthInfoReferenced(c *clientcmdapi.Config, name string) bool {
	for _, ctx := range c.Contexts {
		if ctx != nil && ctx.AuthInfo == name {
			return true
		}
	}
	return false
}

func sortedClusterKeys(m map[string]*clientcmdapi.Cluster) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedAuthInfoKeys(m map[string]*clientcmdapi.AuthInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedContextKeys(m map[string]*clientcmdapi.Context) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
