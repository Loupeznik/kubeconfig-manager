package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// seedStarshipEntry writes a state entry for prod.yaml so buildStarshipLine
// has something to find. Mirrors what the equivalent CLI invocations would do,
// but without routing through cobra — keeps the benchmark measuring the hot
// path rather than flag parsing.
func seedStarshipEntry(b *testing.B, dir string) {
	b.Helper()
	id, err := kubeconfig.IdentifyFile(filepath.Join(dir, "prod.yaml"))
	if err != nil {
		b.Fatal(err)
	}
	store, err := state.DefaultStore()
	if err != nil {
		b.Fatal(err)
	}
	if err := store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.AvailableTags = []string{"prod", "eu"}
		cfg.Entries[id.StableHash] = state.Entry{
			PathHint: "prod.yaml",
			Tags:     []string{"prod", "eu"},
			Alerts:   state.Alerts{Enabled: true, BlockedVerbs: state.DefaultBlockedVerbs()},
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkStarshipColdCache measures the first-prompt cost of `kcm starship`
// against a freshly-loaded state file (store cache cold). Every iteration is a
// standalone buildStarshipLine call so the allocation + file-read cost is part
// of the measurement — which is what starship actually pays, because it invokes
// a new process every prompt.
func BenchmarkStarshipColdCache(b *testing.B) {
	dir := seedKubeconfigDir(b)
	stateHome := b.TempDir()
	isolateState(b, stateHome)
	seedStarshipEntry(b, dir)

	b.Setenv("KUBECONFIG", filepath.Join(dir, "prod.yaml"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildStarshipLine(context.Background(), "", "")
	}
}

// BenchmarkStarshipWarmCache mirrors the cold benchmark but primes the OS page
// cache with a few warm-up iterations before measuring. Serves as a lower
// bound for prompt cost on a system that's been running kcm for a while.
func BenchmarkStarshipWarmCache(b *testing.B) {
	dir := seedKubeconfigDir(b)
	stateHome := b.TempDir()
	isolateState(b, stateHome)
	seedStarshipEntry(b, dir)

	b.Setenv("KUBECONFIG", filepath.Join(dir, "prod.yaml"))
	for i := 0; i < 5; i++ {
		_ = buildStarshipLine(context.Background(), "", "")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildStarshipLine(context.Background(), "", "")
	}
}
