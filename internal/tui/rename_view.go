package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeList
		return m, nil
	case "enter":
		fi, ok := m.currentFileItem()
		if !ok {
			m.mode = modeList
			return m, nil
		}
		newName := strings.TrimSpace(m.renameInput.Value())
		if newName == "" || newName == fi.file.Name() {
			m.mode = modeList
			return m, nil
		}
		if strings.ContainsRune(newName, os.PathSeparator) {
			m.setErr("new name must not contain path separators")
			m.mode = modeList
			return m, nil
		}
		newPath := filepath.Join(filepath.Dir(fi.path), newName)
		if _, err := os.Stat(newPath); err == nil {
			m.setErr(fmt.Sprintf("%s already exists", newName))
			m.mode = modeList
			return m, nil
		}
		if err := os.Rename(fi.path, newPath); err != nil {
			m.setErr("rename failed: " + err.Error())
			m.mode = modeList
			return m, nil
		}
		if err := rebindPathHint(m.store, fi.identity, filepath.Base(newPath)); err != nil {
			m.setErr("rename ok but state update failed: " + err.Error())
			m.mode = modeList
			return m, reloadCmd("")
		}
		m.mode = modeList
		return m, reloadCmd("renamed to " + newName)
	}

	var cmd tea.Cmd
	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
}

func (m Model) viewRename() string {
	fi, ok := m.currentFileItem()
	title := "Rename"
	if ok {
		title = "Rename " + fi.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render(title),
		"",
		"New filename (no path separators):",
		m.renameInput.View(),
		"",
		helpStyle.Render("enter save  esc cancel"),
	)
	return modalBorderStyle.Render(body)
}
