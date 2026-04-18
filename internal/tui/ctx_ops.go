package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) updateCtxRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeDetail
		m.ctxActionName = ""
		return m, nil
	case "enter":
		newName := strings.TrimSpace(m.ctxInput.Value())
		oldName := m.ctxActionName
		m.ctxActionName = ""
		m.ctxInput.Blur()
		if newName == "" || newName == oldName || m.detailFile == nil {
			m.mode = modeDetail
			return m, nil
		}
		if err := renameContextOnDisk(m.store, m.detailFile.path, m.detailFile.identity, oldName, newName); err != nil {
			m.setErr("rename: " + err.Error())
			m.mode = modeDetail
			return m, nil
		}
		m.mode = modeDetail
		return m, reloadCmd(fmt.Sprintf("renamed context %s → %s", oldName, newName))
	}
	var cmd tea.Cmd
	m.ctxInput, cmd = m.ctxInput.Update(msg)
	return m, cmd
}

func (m Model) viewCtxRename() string {
	fileName := "kubeconfig"
	if m.detailFile != nil {
		fileName = m.detailFile.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render("Rename context "+m.ctxActionName+" in "+fileName),
		"",
		"New context name:",
		m.ctxInput.View(),
		"",
		dirHintStyle.Render("Tags, alerts, and current-context (if applicable) are moved to the new name."),
		"",
		renderKey("↵", "save")+separatorStyle.Render(" · ")+renderKey("esc", "cancel"),
	)
	return modalBorderStyle.Render(body)
}

func (m Model) updateCtxDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		name := m.ctxActionName
		m.ctxActionName = ""
		m.mode = modeDetail
		if name == "" || m.detailFile == nil {
			return m, nil
		}
		if err := deleteContextOnDisk(m.store, m.detailFile.path, m.detailFile.identity, name); err != nil {
			m.setErr("delete: " + err.Error())
			return m, nil
		}
		return m, reloadCmd("deleted context " + name)
	case "n", "esc", "ctrl+c":
		m.mode = modeDetail
		m.ctxActionName = ""
		return m, nil
	}
	return m, nil
}

func (m Model) viewCtxDelete() string {
	fileName := "kubeconfig"
	if m.detailFile != nil {
		fileName = m.detailFile.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render("Delete context "+alertBadgeStyle.Render(m.ctxActionName)+" from "+fileName+"?"),
		"",
		detailValueStyle.Render("This removes the context plus its per-context tags and alerts,"),
		detailValueStyle.Render("and prunes the referenced cluster and user if no other context uses them."),
		"",
		renderKey("y", "confirm delete")+separatorStyle.Render(" · ")+renderKey("n/esc", "cancel"),
	)
	return modalBorderStyle.Render(body)
}

func (m Model) updateCtxSplit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeDetail
		m.ctxActionName = ""
		return m, nil
	case "enter":
		outName := strings.TrimSpace(m.ctxInput.Value())
		ctxName := m.ctxActionName
		m.ctxActionName = ""
		m.ctxInput.Blur()
		if outName == "" || m.detailFile == nil {
			m.mode = modeDetail
			return m, nil
		}
		if strings.ContainsRune(outName, os.PathSeparator) {
			m.setErr("output name must not contain path separators")
			m.mode = modeDetail
			return m, nil
		}
		outPath := filepath.Join(filepath.Dir(m.detailFile.path), outName)
		if err := splitContextOnDisk(m.detailFile.path, ctxName, outPath); err != nil {
			m.setErr("split: " + err.Error())
			m.mode = modeDetail
			return m, nil
		}
		m.mode = modeDetail
		return m, reloadCmd(fmt.Sprintf("extracted %s → %s", ctxName, outName))
	}
	var cmd tea.Cmd
	m.ctxInput, cmd = m.ctxInput.Update(msg)
	return m, cmd
}

func (m Model) viewCtxSplit() string {
	fileName := "kubeconfig"
	if m.detailFile != nil {
		fileName = m.detailFile.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render("Extract "+m.ctxActionName+" from "+fileName),
		"",
		"New file name (in the same directory):",
		m.ctxInput.View(),
		"",
		dirHintStyle.Render("The context stays in the source file; a copy is written to the new file."),
		"",
		renderKey("↵", "extract")+separatorStyle.Render(" · ")+renderKey("esc", "cancel"),
	)
	return modalBorderStyle.Render(body)
}
