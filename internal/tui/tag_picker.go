package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// tagPicker is a minimal multi-select list bound to the palette. Space toggles
// the cursor row, enter saves, esc cancels.
type tagPicker struct {
	options  []string
	selected map[string]bool
	cursor   int
}

func newTagPicker(options, initial []string) *tagPicker {
	sel := make(map[string]bool, len(initial))
	for _, t := range initial {
		sel[t] = true
	}
	return &tagPicker{
		options:  append([]string(nil), options...),
		selected: sel,
	}
}

// Update returns (save, cancel) flags. Both false means keep picking.
func (t *tagPicker) Update(msg tea.KeyMsg) (save bool, cancel bool) {
	switch msg.String() {
	case "up", "k":
		if t.cursor > 0 {
			t.cursor--
		}
	case "down", "j":
		if t.cursor < len(t.options)-1 {
			t.cursor++
		}
	case "home":
		t.cursor = 0
	case "end":
		t.cursor = len(t.options) - 1
	case " ", "space", "x":
		if len(t.options) == 0 {
			return false, false
		}
		opt := t.options[t.cursor]
		t.selected[opt] = !t.selected[opt]
	case "a":
		// select all
		for _, o := range t.options {
			t.selected[o] = true
		}
	case "n":
		// select none
		t.selected = map[string]bool{}
	case "enter":
		return true, false
	case "esc", "ctrl+c":
		return false, true
	}
	return false, false
}

// Values returns the selected tags in palette order.
func (t *tagPicker) Values() []string {
	out := make([]string, 0, len(t.selected))
	for _, opt := range t.options {
		if t.selected[opt] {
			out = append(out, opt)
		}
	}
	return out
}

func (t *tagPicker) View() string {
	if len(t.options) == 0 {
		return dirHintStyle.Render("(palette empty — add tags with 'kcm tag palette add <tag...>')")
	}
	var b strings.Builder
	for i, opt := range t.options {
		cursor := "  "
		if i == t.cursor {
			cursor = helpKeyStyle.Render("❯ ")
		}
		check := detailValueStyle.Render("[ ]")
		if t.selected[opt] {
			check = tagStyle.Render("[x]")
		}
		label := detailValueStyle.Render(opt)
		if i == t.cursor {
			label = helpKeyStyle.Render(opt)
		}
		b.WriteString(cursor + check + " " + label + "\n")
	}
	b.WriteString("\n")
	b.WriteString(dirHintStyle.Render("a = all · n = none"))
	return b.String()
}
