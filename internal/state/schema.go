// Package state holds the persistent metadata kcm keeps about each kubeconfig:
// tags, destructive-action alert policies, helm-guard configuration, and the
// global tag palette. schema.go defines the on-disk shape and the resolvers
// that compute effective (per-context, per-file, or global) values.
package state

import (
	"time"

	"gopkg.in/yaml.v3"
)

// CurrentVersion is the state-file schema version this build supports. Loading
// a file with a higher version fails; lower versions are lazily migrated on
// first mutation (see migrate).
const CurrentVersion = 1

// Config is the top-level on-disk document in $XDG_CONFIG_HOME/kubeconfig-manager/config.yaml.
type Config struct {
	Version       int              `yaml:"version"`
	KubeconfigDir string           `yaml:"kubeconfig_dir,omitempty"`
	AvailableTags []string         `yaml:"available_tags,omitempty"`
	HelmGuard     HelmGuard        `yaml:"helm_guard,omitempty"`
	Entries       map[string]Entry `yaml:"entries,omitempty"`
}

// Entry is one kubeconfig's local metadata, keyed in Config.Entries by the
// kubeconfig's stable fingerprint (SHA-256 of its logical topology — see
// internal/kubeconfig.StableHashFile).
type Entry struct {
	PathHint      string              `yaml:"path_hint,omitempty"`
	DisplayName   string              `yaml:"display_name,omitempty"`
	Tags          []string            `yaml:"tags,omitempty"`
	Alerts        Alerts              `yaml:"alerts,omitempty"`
	ContextAlerts map[string]Alerts   `yaml:"context_alerts,omitempty"`
	ContextTags   map[string][]string `yaml:"context_tags,omitempty"`
	HelmGuard     *HelmGuard          `yaml:"helm_guard,omitempty"`
	UpdatedAt     time.Time           `yaml:"updated_at"`
}

// Alerts is the destructive-action guard policy for the kubectl wrapper.
// Applied at two scopes (file-level via Entry.Alerts, per-context via
// Entry.ContextAlerts) with per-context winning — see ResolveAlerts.
type Alerts struct {
	Enabled             bool     `yaml:"enabled"`
	RequireConfirmation bool     `yaml:"require_confirmation,omitempty"`
	ConfirmClusterName  bool     `yaml:"confirm_cluster_name,omitempty"`
	BlockedVerbs        []string `yaml:"blocked_verbs,omitempty"`
}

// HelmGuard configures the helm values-path ↔ context mismatch detector.
// Applied at two scopes with per-entry overriding global (see ResolveHelmGuard).
//
// Enabled is tri-state on purpose: nil means "never configured, use the
// default" (which is ON — the guard is safety-critical and users expect it
// to protect them out of the box), while an explicit pointer to false is the
// result of `kcm helm-guard disable` and overrides the default.
type HelmGuard struct {
	Enabled *bool `yaml:"enabled,omitempty"`
	// Patterns is the ordered list of values-file path templates used to
	// extract a cluster/env name. Each pattern must contain one "{name}"
	// placeholder (capture group stops at the next slash). The first pattern
	// that matches a given values-file path wins. When empty, falls back to
	// the global list (or a single DefaultHelmPattern if global is also empty).
	Patterns []string `yaml:"patterns,omitempty"`
	// GlobalFallback enables a pattern-less fallback: if none of Patterns
	// matches a values-file path, the path itself is tokenized and compared
	// against the active context/cluster tokens. Catches irregular layouts
	// (e.g. "helm/my-app.prod.yaml") without requiring a custom pattern.
	GlobalFallback bool `yaml:"global_fallback,omitempty"`
	// EnvTokens is the set of "environment-like" tokens that, if they appear
	// on one side of the comparison but not the other, mark a high-severity
	// mismatch. When empty, falls back to global then DefaultEnvTokens.
	EnvTokens []string `yaml:"env_tokens,omitempty"`
	// RequireConfirmation reads as true unless explicitly set to false.
	RequireConfirmation bool `yaml:"require_confirmation,omitempty"`
}

// IsEnabled returns the effective Enabled value, defaulting to true when the
// pointer is nil. Call on the resolved HelmGuard (after ResolveHelmGuard),
// not on the raw per-entry or global struct.
func (h HelmGuard) IsEnabled() bool {
	if h.Enabled == nil {
		return true
	}
	return *h.Enabled
}

// BoolPtr is a small helper for constructing *bool literals in state mutators
// and tests. Exported because both internal/cli/helm.go and test code need it.
func BoolPtr(b bool) *bool { return &b }

// UnmarshalYAML accepts the legacy `pattern: "foo"` scalar field (pre-v0.11)
// and folds it into Patterns so on-disk state from earlier versions keeps
// working. Marshaling is never done in the legacy form.
func (h *HelmGuard) UnmarshalYAML(node *yaml.Node) error {
	type raw struct {
		Enabled             *bool    `yaml:"enabled,omitempty"`
		Patterns            []string `yaml:"patterns,omitempty"`
		Pattern             string   `yaml:"pattern,omitempty"`
		GlobalFallback      bool     `yaml:"global_fallback,omitempty"`
		EnvTokens           []string `yaml:"env_tokens,omitempty"`
		RequireConfirmation bool     `yaml:"require_confirmation,omitempty"`
	}
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	h.Enabled = r.Enabled
	h.Patterns = r.Patterns
	if len(h.Patterns) == 0 && r.Pattern != "" {
		h.Patterns = []string{r.Pattern}
	}
	h.GlobalFallback = r.GlobalFallback
	h.EnvTokens = r.EnvTokens
	h.RequireConfirmation = r.RequireConfirmation
	return nil
}

// DefaultHelmPattern is the fallback values-file path template.
const DefaultHelmPattern = "clusters/{name}/"

// DefaultEnvTokens returns the canonical environment-token set used when
// neither per-entry nor global HelmGuard sets one.
func DefaultEnvTokens() []string {
	return []string{
		"prod", "production",
		"staging", "stg", "stage",
		"dev", "development",
		"test", "tst",
		"qa",
		"sandbox", "sbx",
		"preprod", "preview",
	}
}

// DefaultBlockedVerbs is the kubectl verb set the destructive-action guard
// trips on when an Alerts entry has BlockedVerbs unset.
func DefaultBlockedVerbs() []string {
	return []string{"delete", "drain", "cordon", "uncordon", "taint", "replace", "patch"}
}

// NewConfig constructs a fresh, empty Config at the current schema version.
func NewConfig() *Config {
	return &Config{
		Version: CurrentVersion,
		Entries: map[string]Entry{},
	}
}

// ---- resolvers -------------------------------------------------------------

// ResolveHelmGuard returns the effective HelmGuard for this entry, falling
// back from per-entry to global. A per-entry nil struct means "inherit global";
// a per-entry struct with Enabled set to a non-nil false pointer is an
// explicit override to suppress.
//
// Enabled is tri-state: per-entry takes precedence; if per-entry didn't touch
// it, global applies; if neither was set, the default is ON (the guard is a
// safety feature users expect to work out of the box).
//
// GlobalFallback uses OR-semantics (per-entry OR global): either side turning
// it on enables the pattern-less fallback. This avoids a tri-state field for
// a toggle that has no reasonable "explicit disable" use case.
func (e Entry) ResolveHelmGuard(global HelmGuard) HelmGuard {
	base := global
	if e.HelmGuard == nil {
		if base.Enabled == nil {
			base.Enabled = BoolPtr(true)
		}
		if len(base.Patterns) == 0 {
			base.Patterns = []string{DefaultHelmPattern}
		}
		if len(base.EnvTokens) == 0 {
			base.EnvTokens = DefaultEnvTokens()
		}
		return base
	}
	out := *e.HelmGuard
	if out.Enabled == nil {
		out.Enabled = base.Enabled
	}
	if out.Enabled == nil {
		out.Enabled = BoolPtr(true)
	}
	if len(out.Patterns) == 0 {
		out.Patterns = base.Patterns
	}
	if len(out.Patterns) == 0 {
		out.Patterns = []string{DefaultHelmPattern}
	}
	if len(out.EnvTokens) == 0 {
		out.EnvTokens = base.EnvTokens
	}
	if len(out.EnvTokens) == 0 {
		out.EnvTokens = DefaultEnvTokens()
	}
	out.GlobalFallback = out.GlobalFallback || base.GlobalFallback
	return out
}

// ResolveAlerts returns the active alert policy for a given context within
// this entry. A per-context override (if present) wins over the file-level
// policy; if neither is set, the zero Alerts{} is returned.
func (e Entry) ResolveAlerts(contextName string) Alerts {
	if contextName != "" {
		if a, ok := e.ContextAlerts[contextName]; ok {
			return a
		}
	}
	return e.Alerts
}

// ResolveTags returns the effective tag set for a context — union of the
// file-level tags and the context-level tags, deduplicated and order-preserving
// (file-level first, then context-level additions).
func (e Entry) ResolveTags(contextName string) []string {
	seen := make(map[string]bool, len(e.Tags))
	out := make([]string, 0, len(e.Tags))
	for _, t := range e.Tags {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if contextName != "" {
		for _, t := range e.ContextTags[contextName] {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}
