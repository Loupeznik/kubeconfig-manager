package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// renderer is explicitly bound to stderr so lipgloss profiles the terminal
// capability of the render target (stderr), not stdout. When kcm runs through
// the shell hook, stdout is captured by `eval "$(...)"` and would otherwise
// be classified as non-TTY, stripping all color codes from the TUI.
var renderer = lipgloss.NewRenderer(os.Stderr)

var (
	titleStyle = renderer.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	helpStyle = renderer.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	statusStyle = renderer.NewStyle().
			Foreground(lipgloss.Color("34")).
			Padding(0, 1)

	errorStyle = renderer.NewStyle().
			Foreground(lipgloss.Color("196")).
			Padding(0, 1)

	tagStyle = renderer.NewStyle().
			Foreground(lipgloss.Color("#A6E3A1")).
			Bold(true)

	alertBadgeStyle = renderer.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	currentContextStyle = renderer.NewStyle().
				Foreground(lipgloss.Color("#89DCEB"))

	modalBorderStyle = renderer.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7D56F4")).
				Padding(1, 2)

	detailHeaderStyle = renderer.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#F5C2E7"))

	tableHeaderStyle = renderer.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("241"))
)
