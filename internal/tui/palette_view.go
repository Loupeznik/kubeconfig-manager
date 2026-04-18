package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

func paletteListKeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	}
}

func (m *Model) openPalette() {
	// Persist the auto-bootstrap so any subsequent mutation (add/rename/delete)
	// operates on the fully materialized palette rather than an empty one.
	if err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.EnsurePaletteFromEntries()
		return nil
	}); err != nil {
		m.setErr("bootstrap palette: " + err.Error())
		return
	}
	cfg, err := m.store.Load(context.Background())
	if err != nil {
		m.setErr("load state: " + err.Error())
		return
	}
	m.loadPaletteList(cfg)
	m.paletteAction = paletteBrowsing
	m.paletteDelete = ""
	m.paletteRenameFrom = ""
	m.paletteInput.SetValue("")
	m.paletteInput.Blur()
	// Size the palette list; the outer Update will re-size on next window event,
	// but do a sensible fallback so the list shows something immediately.
	if m.width > 0 && m.height > chromeHeight {
		m.paletteList.SetSize(m.width, m.height-chromeHeight)
	}
	m.mode = modePalette
}

// paletteItem is a single tag row in the palette list.
type paletteItem struct {
	tag       string
	locations []string // file / file-context locations where it's used
}

func (p paletteItem) Title() string {
	return tagStyle.Render("●") + " " + p.tag
}

func (p paletteItem) Description() string {
	if len(p.locations) == 0 {
		return detailLabelStyle.Render("unused")
	}
	used := strings.Join(p.locations, ", ")
	if len(used) > 80 {
		used = used[:77] + "..."
	}
	return detailLabelStyle.Render(fmt.Sprintf("used in %d place(s): ", len(p.locations))) + detailValueStyle.Render(used)
}

func (p paletteItem) FilterValue() string { return p.tag }

func (m *Model) loadPaletteList(cfg *state.Config) {
	usage := tagUsage(cfg)
	m.paletteUsage = usage

	items := make([]list.Item, 0, len(cfg.AvailableTags))
	for _, t := range cfg.AvailableTags {
		items = append(items, paletteItem{
			tag:       t,
			locations: usage[t],
		})
	}
	prevIndex := m.paletteList.Index()
	m.paletteList.SetItems(items)
	if prevIndex < len(items) {
		m.paletteList.Select(prevIndex)
	}
}

// tagUsage returns, for each tag, a sorted list of "file" or "file/context"
// locations where it is referenced.
func tagUsage(cfg *state.Config) map[string][]string {
	out := map[string][]string{}
	for _, entry := range cfg.Entries {
		label := entry.PathHint
		if label == "" {
			label = "—"
		}
		for _, t := range entry.Tags {
			out[t] = appendUnique(out[t], label)
		}
		for ctxName, ctxTags := range entry.ContextTags {
			for _, t := range ctxTags {
				out[t] = appendUnique(out[t], label+"/"+ctxName)
			}
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

func appendUnique(xs []string, s string) []string {
	for _, existing := range xs {
		if existing == s {
			return xs
		}
	}
	return append(xs, s)
}

func (m Model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.paletteAction {
	case paletteAdding:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.paletteAction = paletteBrowsing
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			return m, nil
		case "enter":
			tag := strings.TrimSpace(m.paletteInput.Value())
			m.paletteAction = paletteBrowsing
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			if tag == "" {
				return m, nil
			}
			var added []string
			err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
				cfg.EnsurePaletteFromEntries()
				added = cfg.AddAvailableTags(tag)
				return nil
			})
			if err != nil {
				m.setErr("add tag: " + err.Error())
				return m, nil
			}
			msg := "added to palette: " + tag
			if len(added) == 0 {
				msg = tag + " already in palette"
			}
			return m, m.paletteReload(msg)
		}
		var cmd tea.Cmd
		m.paletteInput, cmd = m.paletteInput.Update(msg)
		return m, cmd

	case paletteRenaming:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.paletteAction = paletteBrowsing
			m.paletteRenameFrom = ""
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			return m, nil
		case "enter":
			oldTag := m.paletteRenameFrom
			newTag := strings.TrimSpace(m.paletteInput.Value())
			m.paletteAction = paletteBrowsing
			m.paletteRenameFrom = ""
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			if newTag == "" || newTag == oldTag {
				return m, nil
			}
			err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
				cfg.EnsurePaletteFromEntries()
				return cfg.RenameAvailableTag(oldTag, newTag)
			})
			if err != nil {
				m.setErr("rename tag: " + err.Error())
				return m, nil
			}
			return m, tea.Batch(m.paletteReload(fmt.Sprintf("renamed %s → %s", oldTag, newTag)), reloadCmd(""))
		}
		var cmd tea.Cmd
		m.paletteInput, cmd = m.paletteInput.Update(msg)
		return m, cmd

	case paletteDeleting:
		switch msg.String() {
		case "y":
			victim := m.paletteDelete
			m.paletteAction = paletteBrowsing
			m.paletteDelete = ""
			if victim == "" {
				return m, nil
			}
			var removed []string
			err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
				cfg.EnsurePaletteFromEntries()
				removed = cfg.RemoveAvailableTags(victim)
				return nil
			})
			if err != nil {
				m.setErr("delete tag: " + err.Error())
				return m, nil
			}
			if len(removed) == 0 {
				return m, m.paletteReload(victim + " was not in palette")
			}
			return m, tea.Batch(m.paletteReload("deleted and scrubbed: "+victim), reloadCmd(""))
		case "n", "esc", "ctrl+c":
			m.paletteAction = paletteBrowsing
			m.paletteDelete = ""
			return m, nil
		}
		return m, nil

	default: // paletteBrowsing
		if m.paletteList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.paletteList, cmd = m.paletteList.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.mode = modeList
			m.paletteAction = paletteBrowsing
			m.paletteDelete = ""
			m.paletteRenameFrom = ""
			return m, nil
		case "n":
			m.paletteAction = paletteAdding
			m.paletteInput.SetValue("")
			blink := m.paletteInput.Focus()
			return m, blink
		case "r":
			tag := m.currentPaletteTag()
			if tag == "" {
				return m, nil
			}
			m.paletteAction = paletteRenaming
			m.paletteRenameFrom = tag
			m.paletteInput.SetValue(tag)
			blink := m.paletteInput.Focus()
			return m, blink
		case "d":
			tag := m.currentPaletteTag()
			if tag == "" {
				return m, nil
			}
			m.paletteAction = paletteDeleting
			m.paletteDelete = tag
			return m, nil
		}
		var cmd tea.Cmd
		m.paletteList, cmd = m.paletteList.Update(msg)
		return m, cmd
	}
}

func (m Model) currentPaletteTag() string {
	if m.paletteList.SelectedItem() == nil {
		return ""
	}
	pi, ok := m.paletteList.SelectedItem().(paletteItem)
	if !ok {
		return ""
	}
	return pi.tag
}

func (m Model) paletteReload(status string) tea.Cmd {
	return func() tea.Msg { return paletteReloadMsg{status: status} }
}

type paletteReloadMsg struct {
	status string
}

func (m Model) viewPalette() string {
	title := detailHeaderStyle.Render("Tag palette")
	header := title

	body := m.paletteList.View()

	switch m.paletteAction {
	case paletteAdding:
		body += "\n" + modalBorderStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			detailLabelStyle.Render("New tag name:"),
			m.paletteInput.View(),
		))
	case paletteRenaming:
		body += "\n" + modalBorderStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			detailLabelStyle.Render("Rename "+m.paletteRenameFrom+" to:"),
			m.paletteInput.View(),
			"",
			dirHintStyle.Render("All usages across kubeconfigs and contexts will be updated."),
		))
	case paletteDeleting:
		body += "\n" + modalBorderStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			detailHeaderStyle.Render("Delete tag "+alertBadgeStyle.Render(m.paletteDelete)+"?"),
			detailValueStyle.Render("This also removes it from every kubeconfig and context that uses it."),
		))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}
