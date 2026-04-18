package kubeconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

// Identity is the pair of hashes that can key a kubeconfig in state.
// StableHash is the canonical identifier — built from the file's logical
// topology (clusters, users, contexts) so it survives kctx/kubens/credential
// rotations that rewrite the underlying bytes. ContentHash is the legacy
// byte-hash used by pre-v0.9 state files; it's retained so existing entries
// can be found and migrated to the stable key on first access.
type Identity struct {
	StableHash  string
	ContentHash string
}

// IdentifyFile computes both hashes for a kubeconfig. Call once per file
// access; pass StableHash/ContentHash to state lookups.
func IdentifyFile(path string) (Identity, error) {
	stable, err := StableHashFile(path)
	if err != nil {
		return Identity{}, err
	}
	content, err := ContentHashFile(path)
	if err != nil {
		return Identity{}, err
	}
	return Identity{StableHash: stable, ContentHash: content}, nil
}

// StableHashFile parses the kubeconfig and returns a hash of its logical
// topology (cluster names + servers, user names, context name/cluster/user
// tuples). Stable across:
//   - current-context changes (kubectx / kubectl config use-context)
//   - namespace defaults (kubens / kubectl config set-context --namespace)
//   - credential rotation (new tokens/certs in place)
//   - whitespace/comment/formatting changes
//
// Changes when the set of contexts, clusters, users, or a cluster's server
// URL changes — i.e. real topology shifts.
func StableHashFile(path string) (string, error) {
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return StableFingerprint(cfg), nil
}

// StableFingerprint canonicalizes the kubeconfig into a sorted line-based
// representation and hashes that with SHA-256.
func StableFingerprint(cfg *clientcmdapi.Config) string {
	lines := make([]string, 0, len(cfg.Clusters)+len(cfg.AuthInfos)+len(cfg.Contexts))
	for name, c := range cfg.Clusters {
		server := ""
		if c != nil {
			server = c.Server
		}
		lines = append(lines, "cluster\t"+name+"\t"+server)
	}
	for name := range cfg.AuthInfos {
		lines = append(lines, "user\t"+name)
	}
	for name, c := range cfg.Contexts {
		var cluster, authInfo string
		if c != nil {
			cluster = c.Cluster
			authInfo = c.AuthInfo
		}
		lines = append(lines, "context\t"+name+"\t"+cluster+"\t"+authInfo)
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return "sha256:" + hex.EncodeToString(h[:])
}

// ContentHashFile returns the SHA-256 of the raw file bytes. Pre-stable-hash
// state entries were keyed by this value; retained so they can be migrated
// on first lookup.
func ContentHashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

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
