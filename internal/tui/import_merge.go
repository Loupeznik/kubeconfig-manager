package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
)

func (m Model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.fileInput.Blur()
		m.mode = modeList
		return m, nil
	case "enter":
		srcPath := strings.TrimSpace(m.fileInput.Value())
		m.fileInput.Blur()
		m.mode = modeList
		if srcPath == "" {
			return m, nil
		}
		dest := destForImport(&m)
		if err := importOnDisk(srcPath, dest); err != nil {
			m.setErr("import: " + err.Error())
			return m, nil
		}
		return m, reloadCmd(fmt.Sprintf("imported %s → %s", filepath.Base(srcPath), filepath.Base(dest)))
	}
	var cmd tea.Cmd
	m.fileInput, cmd = m.fileInput.Update(msg)
	return m, cmd
}

// destForImport picks a target file for `i` (import): the currently highlighted
// file if there is one, else the default kubeconfig (~/.kube/config).
func destForImport(m *Model) string {
	if fi, ok := m.currentFileItem(); ok {
		return fi.path
	}
	def, err := kubeconfig.DefaultPath()
	if err != nil {
		return ""
	}
	return def
}

func (m Model) viewImport() string {
	destLabel := "~/.kube/config"
	if fi, ok := m.currentFileItem(); ok {
		destLabel = fi.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render("Import into "+destLabel),
		"",
		"Path to the source kubeconfig to merge in:",
		m.fileInput.View(),
		"",
		dirHintStyle.Render("Uses skip-on-conflict so existing entries are preserved."),
		"",
		renderKey("↵", "import")+separatorStyle.Render(" · ")+renderKey("esc", "cancel"),
	)
	return modalBorderStyle.Render(body)
}

func (m Model) updateMergeSource(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.fileInput.Blur()
		m.mode = modeList
		return m, nil
	case "enter":
		srcB := strings.TrimSpace(m.fileInput.Value())
		if srcB == "" {
			m.fileInput.Blur()
			m.mode = modeList
			return m, nil
		}
		m.mergeSourceB = srcB
		// transition to output-path input
		fi, ok := m.currentFileItem()
		suggested := "merged.yaml"
		if ok {
			suggested = "merged-" + fi.file.Name()
		}
		m.fileInput.SetValue(suggested)
		m.fileInput.Placeholder = "output filename"
		blink := m.fileInput.Focus()
		m.mode = modeMergeOutput
		return m, blink
	}
	var cmd tea.Cmd
	m.fileInput, cmd = m.fileInput.Update(msg)
	return m, cmd
}

func (m Model) viewMergeSource() string {
	sourceA := "~/.kube/config"
	if fi, ok := m.currentFileItem(); ok {
		sourceA = fi.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render("Merge — source A: "+sourceA),
		"",
		"Path to the second source kubeconfig:",
		m.fileInput.View(),
		"",
		dirHintStyle.Render("Both sources remain untouched; a new file is produced."),
		"",
		renderKey("↵", "next")+separatorStyle.Render(" · ")+renderKey("esc", "cancel"),
	)
	return modalBorderStyle.Render(body)
}

func (m Model) updateMergeOutput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.fileInput.Blur()
		m.mergeSourceB = ""
		m.mode = modeList
		return m, nil
	case "enter":
		outName := strings.TrimSpace(m.fileInput.Value())
		srcB := m.mergeSourceB
		m.mergeSourceB = ""
		m.fileInput.Blur()
		m.mode = modeList
		if outName == "" || srcB == "" {
			return m, nil
		}
		fi, ok := m.currentFileItem()
		if !ok {
			m.setErr("merge: no highlighted source")
			return m, nil
		}
		if strings.ContainsRune(outName, os.PathSeparator) {
			m.setErr("merge: output name must not contain path separators")
			return m, nil
		}
		outPath := filepath.Join(filepath.Dir(fi.path), outName)
		if err := mergeOnDisk(fi.path, srcB, outPath); err != nil {
			m.setErr("merge: " + err.Error())
			return m, nil
		}
		return m, reloadCmd(fmt.Sprintf("merged %s + %s → %s", fi.file.Name(), filepath.Base(srcB), outName))
	}
	var cmd tea.Cmd
	m.fileInput, cmd = m.fileInput.Update(msg)
	return m, cmd
}

func (m Model) viewMergeOutput() string {
	sourceA := "(no selection)"
	if fi, ok := m.currentFileItem(); ok {
		sourceA = fi.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render("Merge — output for "+sourceA+" + "+filepath.Base(m.mergeSourceB)),
		"",
		"Output filename (in the same directory as the highlighted source):",
		m.fileInput.View(),
		"",
		dirHintStyle.Render("Uses skip-on-conflict; existing entries in source A win."),
		"",
		renderKey("↵", "merge")+separatorStyle.Render(" · ")+renderKey("esc", "cancel"),
	)
	return modalBorderStyle.Render(body)
}
