package guard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

type Trigger struct {
	Path         string
	Hash         string
	Entry        state.Entry
	Policy       state.Alerts
	ClusterName  string
	ContextName  string
	MatchedVerbs []string
}

type Decision struct {
	Verb     string
	Paths    []string
	Triggers []Trigger
}

func (d Decision) Alert() bool { return len(d.Triggers) > 0 }

func (d Decision) RequireConfirm() bool {
	for _, t := range d.Triggers {
		if t.Policy.RequireConfirmation || t.Policy.ConfirmClusterName {
			return true
		}
	}
	return false
}

func (d Decision) RequireClusterName() bool {
	for _, t := range d.Triggers {
		if t.Policy.ConfirmClusterName {
			return true
		}
	}
	return false
}

func (d Decision) ExpectedClusterNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range d.Triggers {
		if t.ClusterName == "" || seen[t.ClusterName] {
			continue
		}
		seen[t.ClusterName] = true
		out = append(out, t.ClusterName)
	}
	return out
}

func Evaluate(ctx context.Context, store state.Store, kubeconfigEnv string, args []string) (Decision, error) {
	verb := ExtractVerb(args)
	d := Decision{Verb: verb}
	if verb == "" {
		return d, nil
	}

	paths, err := resolveKubeconfigPaths(kubeconfigEnv)
	if err != nil {
		return d, err
	}
	d.Paths = paths
	if len(paths) == 0 {
		return d, nil
	}

	cfg, err := store.Load(ctx)
	if err != nil {
		return d, err
	}

	argContext := ExtractContext(args)

	for _, p := range paths {
		hash, err := kubeconfig.HashFile(p)
		if err != nil {
			continue
		}
		entry, ok := cfg.Entries[hash]
		if !ok {
			continue
		}

		activeContext := argContext
		var clusterName string
		if f, err := kubeconfig.Load(p); err == nil {
			if activeContext == "" {
				activeContext = f.Config.CurrentContext
			}
			if kctx, ok := f.Config.Contexts[activeContext]; ok && kctx != nil {
				clusterName = kctx.Cluster
			}
		}

		policy := entry.ResolveAlerts(activeContext)
		if !policy.Enabled {
			continue
		}
		verbs := policy.BlockedVerbs
		if len(verbs) == 0 {
			verbs = state.DefaultBlockedVerbs()
		}
		if !containsVerb(verbs, verb) {
			continue
		}

		d.Triggers = append(d.Triggers, Trigger{
			Path:         p,
			Hash:         hash,
			Entry:        entry,
			ContextName:  activeContext,
			ClusterName:  clusterName,
			Policy:       policy,
			MatchedVerbs: []string{verb},
		})
	}
	return d, nil
}

func containsVerb(verbs []string, verb string) bool {
	for _, v := range verbs {
		if strings.EqualFold(v, verb) {
			return true
		}
	}
	return false
}

func resolveKubeconfigPaths(kubeconfigEnv string) ([]string, error) {
	if kubeconfigEnv != "" {
		parts := strings.Split(kubeconfigEnv, string(os.PathListSeparator))
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
			out = append(out, p)
		}
		return out, nil
	}
	def, err := kubeconfig.DefaultPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(def); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return []string{def}, nil
}

var ErrDeclined = errors.New("destructive action declined")

func (d Decision) Describe() string {
	if !d.Alert() {
		return ""
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Destructive verb %q will run against:\n", d.Verb)
	for _, t := range d.Triggers {
		_, _ = fmt.Fprintf(&b, "  - %s (context %q, cluster %q, tags: %s)\n",
			t.Path, t.ContextName, t.ClusterName, strings.Join(t.Entry.Tags, ","))
	}
	return b.String()
}
