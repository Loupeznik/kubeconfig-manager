package kubeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type File struct {
	Path   string
	Config *clientcmdapi.Config
}

func (f *File) Name() string {
	return filepath.Base(f.Path)
}

func (f *File) ContextNames() []string {
	names := make([]string, 0, len(f.Config.Contexts))
	for name := range f.Config.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (f *File) ClusterNames() []string {
	names := make([]string, 0, len(f.Config.Clusters))
	for name := range f.Config.Clusters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (f *File) UserNames() []string {
	names := make([]string, 0, len(f.Config.AuthInfos))
	for name := range f.Config.AuthInfos {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Load(path string) (*File, error) {
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	if len(cfg.Contexts) == 0 && len(cfg.Clusters) == 0 && len(cfg.AuthInfos) == 0 {
		return nil, fmt.Errorf("load %s: no contexts, clusters, or users — not a kubeconfig", path)
	}
	return &File{Path: path, Config: cfg}, nil
}

type ScanWarning struct {
	Path string
	Err  error
}

type ScanResult struct {
	Files    []*File
	Warnings []ScanWarning
}

func ScanDir(dir string) (*ScanResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	result := &ScanResult{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		f, err := Load(path)
		if err != nil {
			result.Warnings = append(result.Warnings, ScanWarning{Path: path, Err: err})
			continue
		}
		result.Files = append(result.Files, f)
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})
	return result, nil
}

func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kube"), nil
}

func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config"), nil
}

var ErrNotFound = errors.New("kubeconfig not found")

func ResolvePath(nameOrPath, dir string) (string, error) {
	if strings.HasPrefix(nameOrPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		nameOrPath = filepath.Join(home, nameOrPath[2:])
	}
	if filepath.IsAbs(nameOrPath) || strings.ContainsRune(nameOrPath, os.PathSeparator) {
		if _, err := os.Stat(nameOrPath); err != nil {
			return "", fmt.Errorf("%w: %s", ErrNotFound, nameOrPath)
		}
		return nameOrPath, nil
	}
	for _, candidate := range []string{nameOrPath, nameOrPath + ".yaml", nameOrPath + ".yml"} {
		p := filepath.Join(dir, candidate)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: %q in %s", ErrNotFound, nameOrPath, dir)
}
