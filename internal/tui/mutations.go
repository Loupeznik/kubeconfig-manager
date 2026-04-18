package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// reloadMsg asks the Update loop to re-read files + state from disk. status
// is surfaced as a footer message when non-empty.
type reloadMsg struct {
	status string
}

func reloadCmd(status string) tea.Cmd {
	return func() tea.Msg { return reloadMsg{status: status} }
}

func loadFileItems(ctx context.Context, dir string, store state.Store) ([]list.Item, error) {
	scan, err := kubeconfig.ScanDir(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(scan.Files, func(i, j int) bool {
		return scan.Files[i].Name() < scan.Files[j].Name()
	})

	items := make([]list.Item, 0, len(scan.Files))
	for _, f := range scan.Files {
		id, err := kubeconfig.IdentifyFile(f.Path)
		if err != nil {
			return nil, err
		}
		entry, _ := cfg.GetEntry(id.StableHash, id.ContentHash)
		items = append(items, fileItem{
			path:     f.Path,
			file:     f,
			entry:    entry,
			identity: id,
		})
	}
	return items, nil
}

// refindFile finds the fileItem matching a given path. Path is used instead of
// the stable hash because topology-changing actions (context rename / delete)
// shift the hash but leave the path untouched — matching by hash would cause
// the detail view to go blank after any such action until the user re-enters it.
func refindFile(items []list.Item, path string) *fileItem {
	for _, it := range items {
		fi, ok := it.(fileItem)
		if ok && fi.path == path {
			copy := fi
			return &copy
		}
	}
	return nil
}

// toggleAlert flips alerts for the file (contextName == "") or for a specific
// context within the file.
func toggleAlert(store state.Store, id kubeconfig.Identity, pathHint, contextName string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.TakeEntry(id.StableHash, id.ContentHash)
		entry.PathHint = pathHint
		if contextName == "" {
			entry.Alerts.Enabled = !entry.Alerts.Enabled
			if entry.Alerts.Enabled {
				if !entry.Alerts.RequireConfirmation && !entry.Alerts.ConfirmClusterName {
					entry.Alerts.RequireConfirmation = true
				}
				if len(entry.Alerts.BlockedVerbs) == 0 {
					entry.Alerts.BlockedVerbs = state.DefaultBlockedVerbs()
				}
			}
		} else {
			if entry.ContextAlerts == nil {
				entry.ContextAlerts = map[string]state.Alerts{}
			}
			a := entry.ContextAlerts[contextName]
			a.Enabled = !a.Enabled
			if a.Enabled {
				if !a.RequireConfirmation && !a.ConfirmClusterName {
					a.RequireConfirmation = true
				}
				if len(a.BlockedVerbs) == 0 {
					a.BlockedVerbs = state.DefaultBlockedVerbs()
				}
			}
			entry.ContextAlerts[contextName] = a
		}
		entry.Touch()
		cfg.Entries[id.StableHash] = entry
		return nil
	})
}

// setTags replaces tags at the file level (contextName == "") or for a specific
// context within the file.
func setTags(store state.Store, id kubeconfig.Identity, pathHint, contextName string, tags []string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.TakeEntry(id.StableHash, id.ContentHash)
		entry.PathHint = pathHint
		if contextName == "" {
			entry.Tags = tags
		} else {
			if entry.ContextTags == nil {
				entry.ContextTags = map[string][]string{}
			}
			if len(tags) == 0 {
				delete(entry.ContextTags, contextName)
			} else {
				entry.ContextTags[contextName] = tags
			}
		}
		entry.Touch()
		cfg.Entries[id.StableHash] = entry
		return nil
	})
}

// renameContextOnDisk renames a context within the kubeconfig file and moves
// the per-context state entries to the new name.
func renameContextOnDisk(store state.Store, path string, oldID kubeconfig.Identity, oldName, newName string) error {
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return err
	}
	updated, err := kubeconfig.RenameContext(cfg, oldName, newName)
	if err != nil {
		return err
	}
	if err := clientcmd.WriteToFile(*updated, path); err != nil {
		return err
	}
	newID, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		return err
	}
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.TakeEntry(oldID.StableHash, oldID.ContentHash)
		if entry.ContextAlerts != nil {
			if a, ok := entry.ContextAlerts[oldName]; ok {
				delete(entry.ContextAlerts, oldName)
				entry.ContextAlerts[newName] = a
			}
		}
		if entry.ContextTags != nil {
			if t, ok := entry.ContextTags[oldName]; ok {
				delete(entry.ContextTags, oldName)
				entry.ContextTags[newName] = t
			}
		}
		entry.PathHint = filepath.Base(path)
		entry.Touch()
		cfg.Entries[newID.StableHash] = entry
		return nil
	})
}

// deleteContextOnDisk removes a context from the kubeconfig file (pruning
// orphan cluster/user references) and scrubs the per-context state.
func deleteContextOnDisk(store state.Store, path string, oldID kubeconfig.Identity, name string) error {
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return err
	}
	updated, err := kubeconfig.Remove(cfg, name)
	if err != nil {
		return err
	}
	if err := clientcmd.WriteToFile(*updated, path); err != nil {
		return err
	}
	newID, err := kubeconfig.IdentifyFile(path)
	if err != nil {
		return err
	}
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.TakeEntry(oldID.StableHash, oldID.ContentHash)
		delete(entry.ContextAlerts, name)
		delete(entry.ContextTags, name)
		entry.PathHint = filepath.Base(path)
		entry.Touch()
		cfg.Entries[newID.StableHash] = entry
		return nil
	})
}

// splitContextOnDisk extracts a context into its own file without removing it
// from the source. Refuses to overwrite an existing file.
func splitContextOnDisk(srcPath, contextName, outPath string) error {
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("%s already exists", filepath.Base(outPath))
	}
	cfg, err := clientcmd.LoadFromFile(srcPath)
	if err != nil {
		return err
	}
	extracted, err := kubeconfig.Extract(cfg, contextName)
	if err != nil {
		return err
	}
	return clientcmd.WriteToFile(*extracted, outPath)
}

// importOnDisk merges the kubeconfig at srcPath into destPath (conflict policy
// skip — the destination wins). destPath is created if missing.
func importOnDisk(srcPath, destPath string) error {
	srcCfg, err := clientcmd.LoadFromFile(srcPath)
	if err != nil {
		return fmt.Errorf("load source: %w", err)
	}
	destCfg, err := clientcmd.LoadFromFile(destPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("load dest: %w", err)
		}
		destCfg = nil // Merge creates an empty base when dest is nil
	}
	merged, _, err := kubeconfig.Merge(destCfg, srcCfg, kubeconfig.ConflictSkip)
	if err != nil {
		return err
	}
	return clientcmd.WriteToFile(*merged, destPath)
}

// mergeOnDisk combines two kubeconfigs into a new file (skip-on-conflict;
// source A wins). Refuses to overwrite an existing output.
func mergeOnDisk(aPath, bPath, outPath string) error {
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("%s already exists", filepath.Base(outPath))
	}
	cfgA, err := clientcmd.LoadFromFile(aPath)
	if err != nil {
		return fmt.Errorf("load %s: %w", aPath, err)
	}
	cfgB, err := clientcmd.LoadFromFile(bPath)
	if err != nil {
		return fmt.Errorf("load %s: %w", bPath, err)
	}
	merged, _, err := kubeconfig.Merge(cfgA, cfgB, kubeconfig.ConflictSkip)
	if err != nil {
		return err
	}
	return clientcmd.WriteToFile(*merged, outPath)
}

func rebindPathHint(store state.Store, id kubeconfig.Identity, newHint string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry, ok := cfg.GetEntry(id.StableHash, id.ContentHash)
		if !ok {
			return nil
		}
		entry = cfg.TakeEntry(id.StableHash, id.ContentHash)
		entry.PathHint = newHint
		entry.Touch()
		cfg.Entries[id.StableHash] = entry
		return nil
	})
}
