package guard

import (
	"context"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// activeResolution bundles the pieces both Evaluate (kubectl) and EvaluateHelm
// need per kubeconfig path: the file's identity, the active context/cluster,
// and the state entry (if any).
//
// EntryFound is false when no state entry exists for this file — callers that
// need metadata (kubectl guard) should skip; callers that can fall back to a
// global policy (helm guard) can proceed with the zero Entry.
type activeResolution struct {
	Path        string
	Identity    kubeconfig.Identity
	ContextName string
	ClusterName string
	Entry       state.Entry
	EntryFound  bool
}

// resolveActive loads the kubeconfig at path, determines the active context
// (prefers argContext when non-empty, falls back to the file's
// current-context), and looks up its state entry. Returns ok=false if the
// path can't be hashed (e.g. not a parseable kubeconfig).
func resolveActive(cfg *state.Config, path, argContext string) (activeResolution, bool) {
	id, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		return activeResolution{}, false
	}
	res := activeResolution{Path: path, Identity: id}
	if f, err := kubeconfig.Load(path); err == nil {
		res.ContextName = argContext
		if res.ContextName == "" {
			res.ContextName = f.Config.CurrentContext
		}
		if kctx, ok := f.Config.Contexts[res.ContextName]; ok && kctx != nil {
			res.ClusterName = kctx.Cluster
		}
	}
	res.Entry, res.EntryFound = cfg.GetEntry(id.StableHash, id.ContentHash)
	return res, true
}

// loadStoreAndPaths is the thin top-of-function boilerplate shared by
// Evaluate and EvaluateHelm: expand $KUBECONFIG to a list of paths and load
// the state file.
func loadStoreAndPaths(ctx context.Context, store state.Store, kubeconfigEnv string) ([]string, *state.Config, error) {
	paths, err := resolveKubeconfigPaths(kubeconfigEnv)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := store.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	return paths, cfg, nil
}
