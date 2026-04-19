package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

func ctxListKeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alerts")),
		key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rename")),
		key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete")),
		key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "split")),
	}
}

// contextRow is the in-memory representation of a row in the detail table.
// Kept aligned with the list's Items slice so keybindings can look up the
// per-context data by cursor index.
type contextRow struct {
	name          string
	cluster       string
	user          string
	namespace     string
	isCurrent     bool
	fileTags      []string
	ctxTags       []string
	ctxExclusions []string // file-level tags suppressed for this context
	ctxAlerts     state.Alerts
	fileAlerts    state.Alerts
}

// effectiveTags mirrors state.Entry.ResolveTags: file-level ∪ context-level,
// minus exclusions. Used to pre-populate the tag picker and render badges.
func (r contextRow) effectiveTags() []string {
	excluded := map[string]bool{}
	for _, t := range r.ctxExclusions {
		excluded[t] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, t := range r.fileTags {
		if !seen[t] && !excluded[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range r.ctxTags {
		if !seen[t] && !excluded[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// alertIndicator returns the badge for the effective alert state, or "" when
// there's nothing meaningful to show. "alerts off (override)" only appears
// when the context explicitly suppresses a file-level policy that would
// otherwise apply — showing it when there's no file-level policy in the first
// place would be confusing noise.
func (r contextRow) alertIndicator() string {
	if r.ctxAlerts.Enabled {
		return alertBadgeStyle.Render("⚠ ctx alerts")
	}
	if !r.fileAlerts.Enabled {
		return ""
	}
	if isContextAlertsExplicitlyDisabled(r.ctxAlerts) {
		return detailLabelStyle.Render("alerts off (override)")
	}
	return alertBadgeStyle.Render("⚠ file alerts")
}

func isContextAlertsExplicitlyDisabled(a state.Alerts) bool {
	return !a.Enabled && (a.RequireConfirmation || a.ConfirmClusterName || len(a.BlockedVerbs) > 0)
}

// contextItem is a single context rendered in the detail list. Matches the
// main file list's title/description layout for visual consistency.
type contextItem struct {
	row contextRow
}

func (i contextItem) Title() string {
	if i.row.isCurrent {
		return currentContextStyle.Render("●") + " " + i.row.name
	}
	return "  " + i.row.name
}

func (i contextItem) Description() string {
	sep := separatorStyle.Render(" · ")
	var parts []string
	if i.row.cluster != "" {
		parts = append(parts, detailValueStyle.Render(i.row.cluster))
	}
	if i.row.user != "" {
		parts = append(parts, detailValueStyle.Render(i.row.user))
	}
	if i.row.namespace != "" {
		parts = append(parts, detailValueStyle.Render(i.row.namespace))
	}
	if eff := i.row.effectiveTags(); len(eff) > 0 {
		parts = append(parts, tagStyle.Render(renderTagBadges(eff)))
	}
	if badge := i.row.alertIndicator(); badge != "" {
		parts = append(parts, badge)
	}
	return strings.Join(parts, sep)
}

func (i contextItem) FilterValue() string {
	return i.row.name + " " + strings.Join(i.row.ctxTags, " ")
}

func (m *Model) loadContextList(fi *fileItem, entry state.Entry) {
	rows := buildContextRows(fi, entry)
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, contextItem{row: r})
	}
	m.ctxList.SetItems(items)
}

func (m Model) currentContextRow() (contextRow, bool) {
	if m.ctxList.SelectedItem() == nil {
		return contextRow{}, false
	}
	ci, ok := m.ctxList.SelectedItem().(contextItem)
	if !ok {
		return contextRow{}, false
	}
	return ci.row, true
}

// buildContextRows converts a fileItem + its state entry into the sorted row
// list that feeds the detail list. Returned slice is owned by the caller.
func buildContextRows(fi *fileItem, entry state.Entry) []contextRow {
	names := make([]string, 0, len(fi.file.Config.Contexts))
	for n := range fi.file.Config.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]contextRow, 0, len(names))
	for _, n := range names {
		ctx := fi.file.Config.Contexts[n]
		var cluster, user, ns string
		if ctx != nil {
			cluster = ctx.Cluster
			user = ctx.AuthInfo
			ns = ctx.Namespace
		}
		var ctxTags []string
		if entry.ContextTags != nil {
			ctxTags = entry.ContextTags[n]
		}
		var ctxExclusions []string
		if entry.ContextTagExclusions != nil {
			ctxExclusions = entry.ContextTagExclusions[n]
		}
		var ctxAlerts state.Alerts
		if entry.ContextAlerts != nil {
			ctxAlerts = entry.ContextAlerts[n]
		}
		out = append(out, contextRow{
			name:          n,
			cluster:       cluster,
			user:          user,
			namespace:     ns,
			isCurrent:     n == fi.file.Config.CurrentContext,
			fileTags:      entry.Tags,
			ctxTags:       ctxTags,
			ctxExclusions: ctxExclusions,
			ctxAlerts:     ctxAlerts,
			fileAlerts:    entry.Alerts,
		})
	}
	return out
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Let the list handle keys while actively filtering.
	if m.ctxList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.ctxList, cmd = m.ctxList.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeList
		m.detailFile = nil
		m.targetContext = ""
		return m, nil
	case "t":
		r, ok := m.currentContextRow()
		if !ok || m.detailFile == nil {
			return m, nil
		}
		m.targetContext = r.name
		blink := m.openTagEditor(r.effectiveTags())
		return m, blink
	case "a":
		r, ok := m.currentContextRow()
		if !ok || m.detailFile == nil {
			return m, nil
		}
		entry := m.detailFile.entry
		wasEnabled := entry.ResolveAlerts(r.name).Enabled
		if err := toggleAlert(m.store, m.detailFile.identity, filepath.Base(m.detailFile.path), r.name); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		status := fmt.Sprintf("alerts enabled for context %s", r.name)
		if wasEnabled {
			status = fmt.Sprintf("alerts disabled for context %s", r.name)
		}
		return m, reloadCmd(status)
	case "R":
		r, ok := m.currentContextRow()
		if !ok {
			return m, nil
		}
		m.ctxActionName = r.name
		m.ctxInput.SetValue(r.name)
		blink := m.ctxInput.Focus()
		m.mode = modeCtxRename
		return m, blink
	case "D":
		r, ok := m.currentContextRow()
		if !ok {
			return m, nil
		}
		m.ctxActionName = r.name
		m.mode = modeCtxDelete
		return m, nil
	case "S":
		r, ok := m.currentContextRow()
		if !ok || m.detailFile == nil {
			return m, nil
		}
		m.ctxActionName = r.name
		m.ctxInput.SetValue(r.name + ".yaml")
		blink := m.ctxInput.Focus()
		m.mode = modeCtxSplit
		return m, blink
	}

	var cmd tea.Cmd
	m.ctxList, cmd = m.ctxList.Update(msg)
	return m, cmd
}

func (m Model) viewDetail() string {
	if m.detailFile == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render("(no selection)")
	}

	var headerParts []string
	headerParts = append(headerParts, detailHeaderStyle.Render(m.detailFile.file.Name()))
	if c := m.detailFile.file.Config.CurrentContext; c != "" {
		headerParts = append(headerParts,
			detailLabelStyle.Render("current: ")+currentContextStyle.Render(c))
	}
	if len(m.detailFile.entry.Tags) > 0 {
		headerParts = append(headerParts, tagStyle.Render(renderTagBadges(m.detailFile.entry.Tags)))
	}
	if m.detailFile.entry.Alerts.Enabled {
		headerParts = append(headerParts, alertBadgeStyle.Render("⚠ FILE ALERTS"))
	}
	header := strings.Join(headerParts, separatorStyle.Render(" · "))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.ctxList.View(),
	)
}
