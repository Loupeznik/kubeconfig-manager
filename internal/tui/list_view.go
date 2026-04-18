package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

// fileItem is one kubeconfig row in the main list.
type fileItem struct {
	path     string
	file     *kubeconfig.File
	entry    state.Entry
	identity kubeconfig.Identity
}

func (i fileItem) Title() string { return i.file.Name() }

func (i fileItem) Description() string {
	ctxCount := contextCountStyle.Render(fmt.Sprintf("%d ctx", len(i.file.Config.Contexts)))
	parts := []string{ctxCount}
	if c := i.file.Config.CurrentContext; c != "" {
		parts = append(parts, currentContextStyle.Render("→ "+c))
	}
	if len(i.entry.Tags) > 0 {
		parts = append(parts, tagStyle.Render(renderTagBadges(i.entry.Tags)))
	}
	if i.entry.Alerts.Enabled {
		parts = append(parts, alertBadgeStyle.Render("⚠ ALERT"))
	} else if hasAnyContextAlerts(i.entry) {
		parts = append(parts, alertBadgeStyle.Render("⚠ ctx-alerts"))
	}
	return strings.Join(parts, "  ")
}

func (i fileItem) FilterValue() string {
	return i.file.Name() + " " + strings.Join(i.entry.Tags, " ")
}

func hasAnyContextAlerts(e state.Entry) bool {
	for _, a := range e.ContextAlerts {
		if a.Enabled {
			return true
		}
	}
	return false
}

func renderTagBadges(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tags))
	for _, t := range tags {
		parts = append(parts, "●"+t)
	}
	return strings.Join(parts, " ")
}

func listKeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "select")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "details")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alerts")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
		key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "import")),
		key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "palette")),
	}
}

func (m Model) currentFileItem() (fileItem, bool) {
	if m.fileList.SelectedItem() == nil {
		return fileItem{}, false
	}
	fi, ok := m.fileList.SelectedItem().(fileItem)
	return fi, ok
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.fileList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.fileList, cmd = m.fileList.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "s":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		m.selectedPath = fi.path
		return m, tea.Quit
	case "enter":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		m.detailFile = &fi
		m.loadContextList(&fi, fi.entry)
		m.mode = modeDetail
		return m, nil
	case "t":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		m.targetContext = ""
		blink := m.openTagEditor(fi.entry.Tags)
		return m, blink
	case "r":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		m.renameInput.SetValue(fi.file.Name())
		blink := m.renameInput.Focus()
		m.mode = modeRename
		return m, blink
	case "a":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		wasEnabled := fi.entry.Alerts.Enabled
		if err := toggleAlert(m.store, fi.identity, filepath.Base(fi.path), ""); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		status := "alerts enabled (file-level)"
		if wasEnabled {
			status = "alerts disabled (file-level)"
		}
		return m, reloadCmd(status)
	case "p":
		m.openPalette()
		return m, nil
	case "i":
		m.fileInput.SetValue("")
		m.fileInput.Placeholder = "/path/to/source.yaml"
		blink := m.fileInput.Focus()
		m.mode = modeImport
		return m, blink
	case "m":
		m.fileInput.SetValue("")
		m.fileInput.Placeholder = "/path/to/second-source.yaml"
		blink := m.fileInput.Focus()
		m.mode = modeMergeSource
		return m, blink
	}

	var cmd tea.Cmd
	m.fileList, cmd = m.fileList.Update(msg)
	return m, cmd
}
