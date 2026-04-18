package guard

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// HelmDecision is the helm-specific counterpart to Decision: it captures the
// path-to-context mismatches detected for a `helm` invocation. When empty
// (no triggers) the caller should execute helm unchanged.
type HelmDecision struct {
	ContextName string
	ClusterName string
	Policy      state.HelmGuard
	Triggers    []HelmTrigger
}

// HelmTrigger represents one values-file whose derived name didn't match the
// active context/cluster.
type HelmTrigger struct {
	ValuesPath    string // original -f / --values argument
	DerivedName   string // name extracted via the configured pattern
	ActiveContext string
	ActiveCluster string
	Severity      HelmSeverity
	Reason        string
}

// HelmSeverity ranks the type of mismatch. Callers may want to prompt for the
// "environment" severity unconditionally (token set contradicts: prod vs test)
// while treating "name" severity (no token overlap) as a softer warning.
type HelmSeverity int

const (
	HelmMatchOK   HelmSeverity = iota // derived name matches — no trigger
	HelmMatchSoft                     // no shared tokens but no environment contradiction
	HelmMatchHard                     // environment contradiction (prod vs dev/test/…)
)

func (s HelmSeverity) String() string {
	switch s {
	case HelmMatchSoft:
		return "soft"
	case HelmMatchHard:
		return "hard"
	}
	return "ok"
}

// Alert reports whether any trigger fired.
func (d HelmDecision) Alert() bool { return len(d.Triggers) > 0 }

// Describe renders a human-friendly summary for the confirmation prompt.
func (d HelmDecision) Describe() string {
	if !d.Alert() {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "helm values-path / context mismatch detected:\n")
	fmt.Fprintf(&b, "  active context: %s (cluster %s)\n", d.ContextName, d.ClusterName)
	for _, t := range d.Triggers {
		fmt.Fprintf(&b, "  - %s\n", t.ValuesPath)
		fmt.Fprintf(&b, "      derived name: %q — severity: %s (%s)\n",
			t.DerivedName, t.Severity, t.Reason)
	}
	return b.String()
}

// EvaluateHelm inspects args, extracts values files, derives names via the
// configured pattern, and compares them to the active context's name. Returns
// a HelmDecision with zero triggers when everything lines up.
func EvaluateHelm(ctx context.Context, store state.Store, kubeconfigEnv string, args []string) (HelmDecision, error) {
	d := HelmDecision{}
	paths, cfg, err := loadStoreAndPaths(ctx, store, kubeconfigEnv)
	if err != nil {
		return d, err
	}
	if len(paths) == 0 {
		return d, nil
	}
	// Use the first kubeconfig for context resolution — matches kubectl's
	// default when $KUBECONFIG is a single file and is the common case.
	res, ok := resolveActive(cfg, paths[0], ExtractContext(args))
	if !ok {
		return d, nil
	}
	d.ContextName = res.ContextName
	d.ClusterName = res.ClusterName

	policy := res.Entry.ResolveHelmGuard(cfg.HelmGuard)
	if !policy.Enabled {
		return d, nil
	}
	d.Policy = policy

	for _, vp := range extractHelmValuesPaths(args) {
		derived, ok := deriveNameFromPath(vp, policy.Pattern)
		if !ok {
			continue // path doesn't match the template — silently skip
		}
		sev, reason := compareHelmNames(derived, res.ContextName, res.ClusterName, policy.EnvTokens)
		if sev == HelmMatchOK {
			continue
		}
		d.Triggers = append(d.Triggers, HelmTrigger{
			ValuesPath:    vp,
			DerivedName:   derived,
			ActiveContext: res.ContextName,
			ActiveCluster: res.ClusterName,
			Severity:      sev,
			Reason:        reason,
		})
	}
	return d, nil
}

// extractHelmValuesPaths scans helm args for -f / --values flag values,
// handling both `-f path` and `--values=path` forms, and the `-f a,b,c`
// comma-separated list that helm itself accepts.
func extractHelmValuesPaths(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-f" || a == "--values":
			if i+1 < len(args) {
				out = append(out, splitCommaList(args[i+1])...)
				i++
			}
		case strings.HasPrefix(a, "--values="):
			out = append(out, splitCommaList(strings.TrimPrefix(a, "--values="))...)
		case strings.HasPrefix(a, "-f="):
			out = append(out, splitCommaList(strings.TrimPrefix(a, "-f="))...)
		}
	}
	return out
}

func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// deriveNameFromPath extracts the cluster/env name from a values-file path by
// matching the pattern template. Supports "{name}" as the single placeholder.
// Example:
//
//	pattern: "clusters/{name}/"
//	path:    "envs/clusters/prod-eu/values.yaml"
//	→       ("prod-eu", true)
//
// If the pattern isn't found in the path, returns ("", false).
func deriveNameFromPath(valuesPath, pattern string) (string, bool) {
	// Normalize backslashes to forward slashes unconditionally so the same
	// pattern works on Windows and unix. helm values paths are typically
	// posix-style, but users may pass Windows-style paths too.
	valuesPath = strings.ReplaceAll(filepath.ToSlash(valuesPath), `\`, "/")
	pattern = strings.ReplaceAll(filepath.ToSlash(pattern), `\`, "/")

	const placeholder = "{name}"
	idx := strings.Index(pattern, placeholder)
	if idx < 0 {
		return "", false
	}
	// Build a regex from the pattern by escaping the literal parts and
	// substituting {name} with a capture group that stops at the next slash.
	before := regexp.QuoteMeta(pattern[:idx])
	after := regexp.QuoteMeta(pattern[idx+len(placeholder):])
	re, err := regexp.Compile(before + `([^/]+)` + after)
	if err != nil {
		return "", false
	}
	m := re.FindStringSubmatch(valuesPath)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// compareHelmNames is the core mismatch heuristic. Both names are tokenized
// (split on `-`, `_`, `.`, `/`), lowercased, and compared:
//
//   - If they share an "environment" token (e.g. both contain "prod"),
//     they're considered OK even if other tokens differ — the safety-critical
//     signal lines up.
//   - If one has an env token and the other has a DIFFERENT env token, that's
//     a HARD mismatch (the classic prod-vs-test foot-gun).
//   - If neither has env tokens and the token sets don't overlap at all,
//     that's a SOFT mismatch.
//   - If token sets overlap (even non-env), it's OK.
//
// activeCluster is also compared when the context name alone doesn't match.
func compareHelmNames(derived, context, cluster string, envTokens []string) (HelmSeverity, string) {
	derivedTokens := tokenize(derived)
	ctxTokens := tokenize(context)
	clusterTokens := tokenize(cluster)

	envSet := make(map[string]bool, len(envTokens))
	for _, t := range envTokens {
		envSet[strings.ToLower(t)] = true
	}

	derivedEnv := intersect(derivedTokens, envSet)
	ctxEnv := intersect(ctxTokens, envSet)
	clusterEnv := intersect(clusterTokens, envSet)

	// Hard mismatch: both sides have env tokens, and they don't share any.
	peerEnv := mergeSets(ctxEnv, clusterEnv)
	if len(derivedEnv) > 0 && len(peerEnv) > 0 && !anyIntersection(derivedEnv, peerEnv) {
		return HelmMatchHard, fmt.Sprintf(
			"path environment %v does not match context/cluster environment %v",
			sortedKeys(derivedEnv), sortedKeys(peerEnv))
	}

	// OK: shared env tokens.
	if len(derivedEnv) > 0 && anyIntersection(derivedEnv, peerEnv) {
		return HelmMatchOK, ""
	}

	// OK: any non-env token overlap (e.g. cluster name appears in both).
	allCtx := mergeSlices(ctxTokens, clusterTokens)
	if hasOverlap(derivedTokens, allCtx) {
		return HelmMatchOK, ""
	}

	// Soft mismatch: no overlap at all.
	return HelmMatchSoft, fmt.Sprintf(
		"no overlap between path tokens %v and context tokens %v",
		derivedTokens, allCtx)
}

func tokenize(s string) []string {
	if s == "" {
		return nil
	}
	f := func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '/' || r == ' '
	}
	raw := strings.FieldsFunc(s, f)
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func intersect(tokens []string, set map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, t := range tokens {
		if set[t] {
			out[t] = true
		}
	}
	return out
}

func mergeSets(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

func mergeSlices(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

func anyIntersection(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func hasOverlap(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, t := range a {
		set[t] = true
	}
	for _, t := range b {
		if set[t] {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ExecHelm runs the real helm binary with the given args. Mirrors Exec for
// kubectl.
func ExecHelm(args []string, opts ExecOptions) (int, error) {
	return execByName("helm", args, opts)
}
