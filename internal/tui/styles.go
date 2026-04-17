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

// Kubernetes-inspired palette: kube blue as the accent, white text,
// muted blue-greys for secondary content.
const (
	colorBase     = "#0A1929" // deep kube-navy
	colorText     = "#FFFFFF"
	colorMuted    = "#8AA1C7" // muted blue-grey
	colorSubtle   = "#B8C7E0" // paler blue-grey for hints
	colorAccent   = "#326CE5" // Kubernetes brand blue
	colorAccent2  = "#4F89F0" // lighter kube blue for secondary accents
	colorInfo     = "#60A5FA" // bright blue for current-context marker
	colorSuccess  = "#86E5C1" // muted teal-green for tags (blends with blue)
	colorWarning  = "#FCE38A" // soft yellow
	colorDanger   = "#FF6B6B" // soft red for alert badges
	colorTeal     = "#5EEAD4"
	colorPink     = "#F5BDE6" // kept for section headers
	colorLavender = "#CFE0FF" // light blue-lavender for detail labels
)

var (
	// Header / title
	appTitleStyle = renderer.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorAccent)).
			Padding(0, 1)

	versionStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorMuted)).
			Padding(0, 1)

	dirHintStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorSubtle)).
			Italic(true).
			Padding(0, 1)

	separatorStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorMuted))

	// Status line
	statusStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorSuccess)).
			Padding(0, 1)

	errorStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorDanger)).
			Bold(true).
			Padding(0, 1)

	// Help bar
	helpStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorMuted)).
			Padding(0, 1)

	helpKeyStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorAccent)).
			Bold(true)

	// Badges / markers
	tagStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorSuccess)).
			Bold(true)

	contextTagStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorTeal)).
			Bold(true)

	alertBadgeStyle = renderer.NewStyle().
			Foreground(lipgloss.Color(colorDanger)).
			Bold(true)

	currentContextStyle = renderer.NewStyle().
				Foreground(lipgloss.Color(colorInfo))

	contextCountStyle = renderer.NewStyle().
				Foreground(lipgloss.Color(colorMuted))

	// List title — supplied as l.Styles.Title
	listTitleStyle = renderer.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorAccent)).
			Padding(0, 1)

	// Modal frames
	modalBorderStyle = renderer.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorAccent)).
				Padding(1, 2)

	// Detail view section headers
	detailHeaderStyle = renderer.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorAccent2)).
				Padding(0, 1)

	detailLabelStyle = renderer.NewStyle().
				Foreground(lipgloss.Color(colorLavender)).
				Bold(true)

	detailValueStyle = renderer.NewStyle().
				Foreground(lipgloss.Color(colorText))
)

// renderKey renders a key/description pair like "↵ select" with the key
// highlighted in the accent color.
func renderKey(key, desc string) string {
	return helpKeyStyle.Render(key) + helpStyle.Render(" "+desc)
}
