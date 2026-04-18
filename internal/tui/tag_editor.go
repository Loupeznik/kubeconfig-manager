package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) updateTagEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tagPicker != nil {
		save, cancel := m.tagPicker.Update(msg)
		if cancel {
			m.tagPicker = nil
			m.mode = m.returnModeFromModal()
			m.targetContext = ""
			return m, nil
		}
		if save {
			return m.saveTagsAndReturn(m.tagPicker.Values())
		}
		return m, nil
	}

	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = m.returnModeFromModal()
		m.targetContext = ""
		return m, nil
	case "enter":
		return m.saveTagsAndReturn(splitTags(m.tagInput.Value()))
	}

	var cmd tea.Cmd
	m.tagInput, cmd = m.tagInput.Update(msg)
	return m, cmd
}

func (m Model) saveTagsAndReturn(newTags []string) (tea.Model, tea.Cmd) {
	fi, ok := m.currentFileItem()
	if !ok {
		m.tagPicker = nil
		m.mode = m.returnModeFromModal()
		return m, nil
	}
	if err := setTags(m.store, fi.identity, filepath.Base(fi.path), m.targetContext, newTags); err != nil {
		m.setErr(err.Error())
		m.tagPicker = nil
		m.mode = m.returnModeFromModal()
		return m, nil
	}
	target := "file"
	if m.targetContext != "" {
		target = "context " + m.targetContext
	}
	m.targetContext = ""
	m.tagPicker = nil
	m.mode = m.returnModeFromModal()
	return m, reloadCmd("tags updated (" + target + ")")
}

// openTagEditor picks between the palette-backed multi-select and the
// textinput fallback depending on whether the palette has any tags. Returns
// the textinput blink command when the fallback is used, so the caller can
// surface it as a tea.Cmd and the cursor blinks.
func (m *Model) openTagEditor(currentTags []string) tea.Cmd {
	cfg, err := m.store.Load(context.Background())
	if err == nil {
		cfg.EnsurePaletteFromEntries()
		if len(cfg.AvailableTags) > 0 {
			m.tagPicker = newTagPicker(cfg.AvailableTags, currentTags)
			m.mode = modeTagEdit
			return nil
		}
	}
	m.tagPicker = nil
	m.tagInput.SetValue(strings.Join(currentTags, ", "))
	blink := m.tagInput.Focus()
	m.mode = modeTagEdit
	return blink
}

func (m Model) returnModeFromModal() mode {
	if m.detailFile != nil {
		return modeDetail
	}
	return modeList
}

func (m Model) viewTagEdit() string {
	fi, ok := m.currentFileItem()
	title := "Edit tags"
	if ok {
		if m.targetContext != "" {
			title = fmt.Sprintf("Tags for context %s (in %s)", m.targetContext, fi.file.Name())
		} else {
			title = "File-level tags for " + fi.file.Name()
		}
	}

	if m.tagPicker != nil {
		body := lipgloss.JoinVertical(
			lipgloss.Left,
			detailHeaderStyle.Render(title),
			"",
			dirHintStyle.Render("Select tags from the palette:"),
			m.tagPicker.View(),
			"",
			renderKey("space", "toggle")+separatorStyle.Render(" · ")+
				renderKey("↵", "save")+separatorStyle.Render(" · ")+
				renderKey("esc", "cancel"),
		)
		return modalBorderStyle.Render(body)
	}

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render(title),
		"",
		dirHintStyle.Render("Palette empty — enter comma-separated tags:"),
		m.tagInput.View(),
		"",
		dirHintStyle.Render("Tip: populate the palette with `kcm tag palette add <tag...>` for a multi-select picker."),
		"",
		renderKey("↵", "save")+separatorStyle.Render(" · ")+renderKey("esc", "cancel"),
	)
	return modalBorderStyle.Render(body)
}

// splitTags parses a comma-separated free-text tag list from the fallback
// input, trimming blanks.
func splitTags(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
