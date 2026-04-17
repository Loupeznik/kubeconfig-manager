package kubeconfig

import (
	"errors"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func configFixture(name, cluster, user string) *clientcmdapi.Config {
	return &clientcmdapi.Config{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters: map[string]*clientcmdapi.Cluster{
			cluster: {Server: "https://" + cluster + ".example"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			user: {Token: "token-" + user},
		},
		Contexts: map[string]*clientcmdapi.Context{
			name: {Cluster: cluster, AuthInfo: user, Namespace: "default"},
		},
		CurrentContext: name,
	}
}

func TestMergeDisjoint(t *testing.T) {
	a := configFixture("prod", "prod-c", "prod-u")
	b := configFixture("staging", "staging-c", "staging-u")

	merged, col, err := Merge(a, b, ConflictError)
	if err != nil {
		t.Fatal(err)
	}
	if !col.IsEmpty() {
		t.Errorf("unexpected collisions: %+v", col)
	}
	if len(merged.Clusters) != 2 || len(merged.AuthInfos) != 2 || len(merged.Contexts) != 2 {
		t.Errorf("merged counts wrong: clusters=%d authinfos=%d contexts=%d",
			len(merged.Clusters), len(merged.AuthInfos), len(merged.Contexts))
	}
	if merged.CurrentContext != "prod" {
		t.Errorf("current-context: got %q, want prod", merged.CurrentContext)
	}
}

func TestMergeCollisionError(t *testing.T) {
	a := configFixture("prod", "prod-c", "prod-u")
	b := configFixture("prod", "prod-c", "prod-u")

	_, _, err := Merge(a, b, ConflictError)
	if err == nil {
		t.Fatal("expected collision error")
	}
	var col Collisions
	if !errors.As(err, &col) {
		t.Fatalf("expected Collisions error, got %T", err)
	}
	if len(col.Contexts) != 1 || col.Contexts[0] != "prod" {
		t.Errorf("context collision: %v", col.Contexts)
	}
}

func TestMergeCollisionSkipKeepsDestination(t *testing.T) {
	a := configFixture("prod", "prod-c", "prod-u")
	a.Clusters["prod-c"].Server = "https://DEST-WINS"
	b := configFixture("prod", "prod-c", "prod-u")
	b.Clusters["prod-c"].Server = "https://SRC-SHOULD-SKIP"

	merged, _, err := Merge(a, b, ConflictSkip)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.Clusters["prod-c"].Server; got != "https://DEST-WINS" {
		t.Errorf("skip policy should keep destination: got %q", got)
	}
}

func TestMergeCollisionOverwriteTakesSource(t *testing.T) {
	a := configFixture("prod", "prod-c", "prod-u")
	a.Clusters["prod-c"].Server = "https://dest"
	b := configFixture("prod", "prod-c", "prod-u")
	b.Clusters["prod-c"].Server = "https://SRC-WINS"

	merged, _, err := Merge(a, b, ConflictOverwrite)
	if err != nil {
		t.Fatal(err)
	}
	if got := merged.Clusters["prod-c"].Server; got != "https://SRC-WINS" {
		t.Errorf("overwrite policy should take source: got %q", got)
	}
}

func TestMergeNilDestinationCreatesEmpty(t *testing.T) {
	b := configFixture("prod", "prod-c", "prod-u")
	merged, _, err := Merge(nil, b, ConflictError)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Contexts) != 1 {
		t.Errorf("expected 1 context, got %d", len(merged.Contexts))
	}
	if merged.CurrentContext != "prod" {
		t.Errorf("current-context lost: got %q", merged.CurrentContext)
	}
}

func TestExtractHappyPath(t *testing.T) {
	src := configFixture("prod", "prod-c", "prod-u")
	src.Clusters["staging-c"] = &clientcmdapi.Cluster{Server: "https://staging"}
	src.AuthInfos["staging-u"] = &clientcmdapi.AuthInfo{Token: "s"}
	src.Contexts["staging"] = &clientcmdapi.Context{Cluster: "staging-c", AuthInfo: "staging-u"}

	out, err := Extract(src, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Contexts) != 1 || out.Contexts["prod"] == nil {
		t.Errorf("expected only prod context: %v", out.Contexts)
	}
	if len(out.Clusters) != 1 || out.Clusters["prod-c"] == nil {
		t.Errorf("expected only prod-c cluster: %v", out.Clusters)
	}
	if len(out.AuthInfos) != 1 || out.AuthInfos["prod-u"] == nil {
		t.Errorf("expected only prod-u user: %v", out.AuthInfos)
	}
	if out.CurrentContext != "prod" {
		t.Errorf("current-context: got %q", out.CurrentContext)
	}
}

func TestExtractMissingContext(t *testing.T) {
	src := configFixture("prod", "prod-c", "prod-u")
	if _, err := Extract(src, "nonexistent"); err == nil {
		t.Fatal("expected error for missing context")
	}
}

func TestExtractDanglingClusterReferenceFails(t *testing.T) {
	src := configFixture("prod", "prod-c", "prod-u")
	delete(src.Clusters, "prod-c")
	if _, err := Extract(src, "prod"); err == nil {
		t.Fatal("expected error for missing cluster reference")
	}
}

func TestRemoveContext(t *testing.T) {
	src := configFixture("prod", "prod-c", "prod-u")
	src.Contexts["staging"] = &clientcmdapi.Context{Cluster: "prod-c", AuthInfo: "prod-u"}

	pruned, err := Remove(src, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pruned.Contexts["prod"]; ok {
		t.Error("prod context should be removed")
	}
	if _, ok := pruned.Clusters["prod-c"]; !ok {
		t.Error("prod-c cluster should remain because staging references it")
	}
	if pruned.CurrentContext != "" {
		t.Errorf("current-context: got %q, want empty", pruned.CurrentContext)
	}
}

func TestRemoveContextPrunesOrphans(t *testing.T) {
	src := configFixture("prod", "prod-c", "prod-u")
	pruned, err := Remove(src, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pruned.Clusters["prod-c"]; ok {
		t.Error("prod-c cluster should be pruned (no other refs)")
	}
	if _, ok := pruned.AuthInfos["prod-u"]; ok {
		t.Error("prod-u user should be pruned (no other refs)")
	}
}

func TestParseConflictPolicy(t *testing.T) {
	cases := map[string]ConflictPolicy{
		"":          ConflictError,
		"error":     ConflictError,
		"skip":      ConflictSkip,
		"overwrite": ConflictOverwrite,
	}
	for in, want := range cases {
		got, err := ParseConflictPolicy(in)
		if err != nil {
			t.Errorf("ParseConflictPolicy(%q) err=%v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseConflictPolicy(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseConflictPolicy("bogus"); err == nil {
		t.Error("expected error for unknown policy")
	}
}
