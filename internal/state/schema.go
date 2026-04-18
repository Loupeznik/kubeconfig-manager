// Package state holds the persistent metadata kcm keeps about each kubeconfig:
// tags, destructive-action alert policies, helm-guard configuration, and the
// global tag palette. schema.go defines the on-disk shape and the resolvers
// that compute effective (per-context, per-file, or global) values.
package state

import "time"

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
type HelmGuard struct {
	Enabled bool `yaml:"enabled"`
	// Pattern is the values-file path template used to extract a name, e.g.
	// "clusters/{name}/" matches ".../clusters/prod-eu/values.yaml" and
	// produces "prod-eu". When empty, falls back to the global pattern (or
	// DefaultHelmPattern if global is also empty).
	Pattern string `yaml:"pattern,omitempty"`
	// EnvTokens is the set of "environment-like" tokens that, if they appear
	// on one side of the comparison but not the other, mark a high-severity
	// mismatch. When empty, falls back to global then DefaultEnvTokens.
	EnvTokens []string `yaml:"env_tokens,omitempty"`
	// RequireConfirmation reads as true unless explicitly set to false.
	RequireConfirmation bool `yaml:"require_confirmation,omitempty"`
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
// back from per-entry to global. A per-entry nil means "inherit global";
// a per-entry struct with Enabled=false is an explicit override to suppress.
func (e Entry) ResolveHelmGuard(global HelmGuard) HelmGuard {
	base := global
	if e.HelmGuard == nil {
		if base.Pattern == "" {
			base.Pattern = DefaultHelmPattern
		}
		if len(base.EnvTokens) == 0 {
			base.EnvTokens = DefaultEnvTokens()
		}
		return base
	}
	out := *e.HelmGuard
	if out.Pattern == "" {
		out.Pattern = base.Pattern
	}
	if out.Pattern == "" {
		out.Pattern = DefaultHelmPattern
	}
	if len(out.EnvTokens) == 0 {
		out.EnvTokens = base.EnvTokens
	}
	if len(out.EnvTokens) == 0 {
		out.EnvTokens = DefaultEnvTokens()
	}
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
