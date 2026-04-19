package state

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---- Entry mutations -------------------------------------------------------

// Touch stamps Entry.UpdatedAt with the current UTC time. Callers should Touch
// before writing the entry back so Store implementations (future cloud sync)
// can use last-writer-wins.
func (e *Entry) Touch() {
	e.UpdatedAt = time.Now().UTC()
}

// AddTags inserts tags into the entry's file-level tag list, deduping and
// normalizing whitespace. Returns the list of newly-added tags.
func (e *Entry) AddTags(tags ...string) (added []string) {
	seen := map[string]bool{}
	for _, t := range e.Tags {
		seen[t] = true
	}
	for _, t := range tags {
		t = normalizeTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		e.Tags = append(e.Tags, t)
		added = append(added, t)
	}
	return added
}

// RemoveTags drops tags from the entry's file-level tag list. Returns the
// subset that was actually present.
func (e *Entry) RemoveTags(tags ...string) (removed []string) {
	drop := map[string]bool{}
	for _, t := range tags {
		drop[normalizeTag(t)] = true
	}
	kept := e.Tags[:0]
	for _, t := range e.Tags {
		if drop[t] {
			removed = append(removed, t)
			continue
		}
		kept = append(kept, t)
	}
	e.Tags = kept
	return removed
}

// AddContextTags adds unique tags to the given context's tag list on the Entry.
func (e *Entry) AddContextTags(contextName string, tags ...string) (added []string) {
	if e.ContextTags == nil {
		e.ContextTags = map[string][]string{}
	}
	existing := e.ContextTags[contextName]
	seen := map[string]bool{}
	for _, t := range existing {
		seen[t] = true
	}
	for _, t := range tags {
		t = normalizeTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		existing = append(existing, t)
		added = append(added, t)
	}
	e.ContextTags[contextName] = existing
	return added
}

// RemoveContextTags removes tags from the given context's tag list.
func (e *Entry) RemoveContextTags(contextName string, tags ...string) (removed []string) {
	if e.ContextTags == nil {
		return nil
	}
	drop := map[string]bool{}
	for _, t := range tags {
		drop[normalizeTag(t)] = true
	}
	existing := e.ContextTags[contextName]
	kept := existing[:0]
	for _, t := range existing {
		if drop[t] {
			removed = append(removed, t)
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		delete(e.ContextTags, contextName)
	} else {
		e.ContextTags[contextName] = kept
	}
	return removed
}

// ---- Config lookup & migration helpers -------------------------------------

// GetEntry finds an entry by its stable key. If not found, falls back to
// the legacy (content-hash) key as a read-only migration aid. Returns the
// zero Entry and false if neither key is present. Does not mutate the map.
func (c *Config) GetEntry(stableKey, legacyKey string) (Entry, bool) {
	if e, ok := c.Entries[stableKey]; ok {
		return e, true
	}
	if legacyKey != "" && legacyKey != stableKey {
		if e, ok := c.Entries[legacyKey]; ok {
			return e, true
		}
	}
	return Entry{}, false
}

// TakeEntry returns the entry for the given file identity, transparently
// migrating legacy-keyed entries to the stable key. Call inside a Mutate
// callback before making changes; the caller is expected to write the
// returned entry back under stableKey.
func (c *Config) TakeEntry(stableKey, legacyKey string) Entry {
	if e, ok := c.Entries[stableKey]; ok {
		return e
	}
	if legacyKey != "" && legacyKey != stableKey {
		if e, ok := c.Entries[legacyKey]; ok {
			delete(c.Entries, legacyKey)
			return e
		}
	}
	return Entry{}
}

// ---- Palette (tag allow-list) ----------------------------------------------

// AddAvailableTags inserts unique tags into the palette (order-preserving).
func (c *Config) AddAvailableTags(tags ...string) (added []string) {
	seen := map[string]bool{}
	for _, t := range c.AvailableTags {
		seen[t] = true
	}
	for _, t := range tags {
		t = normalizeTag(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		c.AvailableTags = append(c.AvailableTags, t)
		added = append(added, t)
	}
	return added
}

// RemoveAvailableTags drops tags from the palette; also scrubs them from every
// entry's file-level and per-context tag lists so listings stay consistent.
func (c *Config) RemoveAvailableTags(tags ...string) (removed []string) {
	drop := map[string]bool{}
	for _, t := range tags {
		drop[normalizeTag(t)] = true
	}
	kept := c.AvailableTags[:0]
	for _, t := range c.AvailableTags {
		if drop[t] {
			removed = append(removed, t)
			continue
		}
		kept = append(kept, t)
	}
	c.AvailableTags = kept

	// Scrub from entries.
	for hash, entry := range c.Entries {
		fileKept := entry.Tags[:0]
		for _, t := range entry.Tags {
			if !drop[t] {
				fileKept = append(fileKept, t)
			}
		}
		entry.Tags = fileKept
		for ctxName, ctxTags := range entry.ContextTags {
			ctxKept := ctxTags[:0]
			for _, t := range ctxTags {
				if !drop[t] {
					ctxKept = append(ctxKept, t)
				}
			}
			if len(ctxKept) == 0 {
				delete(entry.ContextTags, ctxName)
			} else {
				entry.ContextTags[ctxName] = ctxKept
			}
		}
		for ctxName, excl := range entry.ContextTagExclusions {
			kept := excl[:0]
			for _, t := range excl {
				if !drop[t] {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(entry.ContextTagExclusions, ctxName)
			} else {
				entry.ContextTagExclusions[ctxName] = kept
			}
		}
		c.Entries[hash] = entry
	}
	return removed
}

// RenameAvailableTag renames a palette tag and updates every entry that
// references the old name (both file-level and per-context). Returns an error
// if the new name is empty, already present, or the old name is not found.
func (c *Config) RenameAvailableTag(oldTag, newTag string) error {
	oldTag = normalizeTag(oldTag)
	newTag = normalizeTag(newTag)
	if newTag == "" {
		return errors.New("new tag name cannot be empty")
	}
	if oldTag == newTag {
		return nil
	}
	for _, t := range c.AvailableTags {
		if t == newTag {
			return fmt.Errorf("tag %q already exists in palette", newTag)
		}
	}
	idx := -1
	for i, t := range c.AvailableTags {
		if t == oldTag {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("tag %q not found in palette", oldTag)
	}
	c.AvailableTags[idx] = newTag

	for hash, entry := range c.Entries {
		for i, t := range entry.Tags {
			if t == oldTag {
				entry.Tags[i] = newTag
			}
		}
		for ctxName, ctxTags := range entry.ContextTags {
			for i, t := range ctxTags {
				if t == oldTag {
					ctxTags[i] = newTag
				}
			}
			entry.ContextTags[ctxName] = ctxTags
		}
		c.Entries[hash] = entry
	}
	return nil
}

// IsTagInPalette reports whether tag (after whitespace trimming) is present in
// the global palette.
func (c *Config) IsTagInPalette(tag string) bool {
	tag = normalizeTag(tag)
	for _, t := range c.AvailableTags {
		if t == tag {
			return true
		}
	}
	return false
}

// EnsurePaletteFromEntries makes sure every tag attached to any entry (file
// level or per-context) is present in the palette. Adds any missing tag,
// preserves existing palette order, dedupes. Safe to call repeatedly — it is
// both a first-run bootstrap and a repair step for state modified by older
// versions, `--allow-new` flows, or direct edits that bypass the palette.
//
// Returns the list of tags newly added to the palette.
func (c *Config) EnsurePaletteFromEntries() (added []string) {
	inPalette := make(map[string]bool, len(c.AvailableTags))
	for _, t := range c.AvailableTags {
		inPalette[t] = true
	}
	addNew := func(t string) {
		if t == "" || inPalette[t] {
			return
		}
		inPalette[t] = true
		c.AvailableTags = append(c.AvailableTags, t)
		added = append(added, t)
	}
	for _, entry := range c.Entries {
		for _, t := range entry.Tags {
			addNew(t)
		}
		for _, ctxTags := range entry.ContextTags {
			for _, t := range ctxTags {
				addNew(t)
			}
		}
	}
	return added
}

// normalizeTag trims whitespace. Kept as its own helper so we can extend it
// later (lowercase? strict ASCII?) without chasing every call site.
func normalizeTag(t string) string {
	return strings.TrimSpace(t)
}
