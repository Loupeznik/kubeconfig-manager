package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/loupeznik/kubeconfig-manager/internal/kubeconfig"
	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

type mode int

const (
	modeList mode = iota
	modeDetail
	modeTagEdit
	modeRename
)

type Model struct {
	mode        mode
	dir         string
	store       state.Store
	list        list.Model
	tagInput    textinput.Model
	renameInput textinput.Model
	detail      *fileItem
	width       int
	height      int
	status      string
	statusErr   bool

	selectedPath string
}

func Run(ctx context.Context, dir string, store state.Store) (string, error) {
	m, err := newModel(ctx, dir, store)
	if err != nil {
		return "", err
	}

	prog := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithOutput(os.Stderr),
		tea.WithAltScreen(),
	)
	final, err := prog.Run()
	if err != nil {
		return "", err
	}
	mm, ok := final.(Model)
	if !ok {
		return "", fmt.Errorf("unexpected final model type: %T", final)
	}
	return mm.selectedPath, nil
}

func newModel(ctx context.Context, dir string, store state.Store) (Model, error) {
	items, err := loadItems(ctx, dir, store)
	if err != nil {
		return Model{}, err
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "kubeconfig-manager"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle
	l.SetStatusBarItemName("kubeconfig", "kubeconfigs")
	l.AdditionalShortHelpKeys = extraKeyBindings
	l.AdditionalFullHelpKeys = extraKeyBindings

	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 512

	ri := textinput.New()
	ri.Prompt = "> "
	ri.CharLimit = 255

	return Model{
		mode:        modeList,
		dir:         dir,
		store:       store,
		list:        l,
		tagInput:    ti,
		renameInput: ri,
	}, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeList:
			return m.updateList(msg)
		case modeDetail:
			return m.updateDetail(msg)
		case modeTagEdit:
			return m.updateTagEdit(msg)
		case modeRename:
			return m.updateRename(msg)
		}

	case reloadMsg:
		items, err := loadItems(context.Background(), m.dir, m.store)
		if err != nil {
			m.setErr(fmt.Sprintf("reload failed: %v", err))
			return m, nil
		}
		m.list.SetItems(items)
		if msg.status != "" {
			m.setStatus(msg.status)
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.mode {
	case modeList:
		m.list, cmd = m.list.Update(msg)
	case modeTagEdit:
		m.tagInput, cmd = m.tagInput.Update(msg)
	case modeRename:
		m.renameInput, cmd = m.renameInput.Update(msg)
	}
	return m, cmd
}

func (m Model) View() string {
	switch m.mode {
	case modeList:
		return m.viewList()
	case modeDetail:
		return m.viewDetail()
	case modeTagEdit:
		return m.viewTagEdit()
	case modeRename:
		return m.viewRename()
	}
	return ""
}

func (m *Model) setStatus(s string) {
	m.status = s
	m.statusErr = false
}

func (m *Model) setErr(s string) {
	m.status = s
	m.statusErr = true
}

func (m Model) renderFooter(keys string) string {
	var parts []string
	parts = append(parts, helpStyle.Render(keys))
	if m.status != "" {
		style := statusStyle
		if m.statusErr {
			style = errorStyle
		}
		parts = append(parts, style.Render(m.status))
	}
	return strings.Join(parts, "  ")
}

type reloadMsg struct {
	status string
}

func reloadCmd(status string) tea.Cmd {
	return func() tea.Msg { return reloadMsg{status: status} }
}

func extraKeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "select")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle alerts")),
	}
}

type fileItem struct {
	path  string
	file  *kubeconfig.File
	entry state.Entry
	hash  string
}

func (i fileItem) Title() string {
	return i.file.Name()
}

func (i fileItem) Description() string {
	parts := []string{
		fmt.Sprintf("%d ctx", len(i.file.Config.Contexts)),
	}
	if c := i.file.Config.CurrentContext; c != "" {
		parts = append(parts, currentContextStyle.Render(c))
	}
	if len(i.entry.Tags) > 0 {
		parts = append(parts, tagStyle.Render(strings.Join(i.entry.Tags, ",")))
	}
	if i.entry.Alerts.Enabled {
		parts = append(parts, alertBadgeStyle.Render("ALERT"))
	}
	return strings.Join(parts, "  ")
}

func (i fileItem) FilterValue() string {
	return i.file.Name() + " " + strings.Join(i.entry.Tags, " ")
}

func loadItems(ctx context.Context, dir string, store state.Store) ([]list.Item, error) {
	scan, err := kubeconfig.ScanDir(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(scan.Files, func(i, j int) bool {
		return scan.Files[i].Name() < scan.Files[j].Name()
	})

	items := make([]list.Item, 0, len(scan.Files))
	for _, f := range scan.Files {
		hash, err := kubeconfig.HashFile(f.Path)
		if err != nil {
			return nil, err
		}
		items = append(items, fileItem{
			path:  f.Path,
			file:  f,
			entry: cfg.Entries[hash],
			hash:  hash,
		})
	}
	return items, nil
}

func (m Model) currentItem() (fileItem, bool) {
	if m.list.SelectedItem() == nil {
		return fileItem{}, false
	}
	fi, ok := m.list.SelectedItem().(fileItem)
	return fi, ok
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "enter":
		fi, ok := m.currentItem()
		if !ok {
			return m, nil
		}
		m.detail = &fi
		m.mode = modeDetail
		return m, nil
	case "x":
		fi, ok := m.currentItem()
		if !ok {
			return m, nil
		}
		m.selectedPath = fi.path
		return m, tea.Quit
	case "t":
		fi, ok := m.currentItem()
		if !ok {
			return m, nil
		}
		m.tagInput.SetValue(strings.Join(fi.entry.Tags, ", "))
		m.tagInput.Focus()
		m.mode = modeTagEdit
		return m, nil
	case "r":
		fi, ok := m.currentItem()
		if !ok {
			return m, nil
		}
		m.renameInput.SetValue(fi.file.Name())
		m.renameInput.Focus()
		m.mode = modeRename
		return m, nil
	case "a":
		fi, ok := m.currentItem()
		if !ok {
			return m, nil
		}
		if err := toggleAlert(m.store, fi.hash, filepath.Base(fi.path)); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		nextStatus := "alerts disabled"
		if !fi.entry.Alerts.Enabled {
			nextStatus = "alerts enabled"
		}
		return m, reloadCmd(nextStatus + " for " + fi.file.Name())
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		m.mode = modeList
		m.detail = nil
		return m, nil
	case "x":
		if m.detail != nil {
			m.selectedPath = m.detail.path
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updateTagEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeList
		return m, nil
	case "enter":
		fi, ok := m.currentItem()
		if !ok {
			m.mode = modeList
			return m, nil
		}
		newTags := splitTags(m.tagInput.Value())
		if err := setTags(m.store, fi.hash, filepath.Base(fi.path), newTags); err != nil {
			m.setErr(err.Error())
			m.mode = modeList
			return m, nil
		}
		m.mode = modeList
		return m, reloadCmd("tags updated for " + fi.file.Name())
	}

	var cmd tea.Cmd
	m.tagInput, cmd = m.tagInput.Update(msg)
	return m, cmd
}

func (m Model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeList
		return m, nil
	case "enter":
		fi, ok := m.currentItem()
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
		if err := rebindPathHint(m.store, fi.hash, filepath.Base(newPath)); err != nil {
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

func (m Model) viewList() string {
	return m.list.View()
}

func (m Model) viewDetail() string {
	if m.detail == nil {
		return "(no selection)"
	}
	f := m.detail.file

	var b strings.Builder
	b.WriteString(detailHeaderStyle.Render(f.Name()))
	b.WriteString("\n\n")
	_, _ = fmt.Fprintf(&b, "Path:     %s\n", f.Path)
	current := f.Config.CurrentContext
	if current == "" {
		current = "-"
	}
	_, _ = fmt.Fprintf(&b, "Current:  %s\n", currentContextStyle.Render(current))
	if len(m.detail.entry.Tags) > 0 {
		_, _ = fmt.Fprintf(&b, "Tags:     %s\n", tagStyle.Render(strings.Join(m.detail.entry.Tags, ", ")))
	}
	if m.detail.entry.Alerts.Enabled {
		_, _ = fmt.Fprintf(&b, "Alerts:   %s\n", alertBadgeStyle.Render("ENABLED"))
	}
	b.WriteString("\n")

	b.WriteString(tableHeaderStyle.Render("Contexts"))
	b.WriteString("\n")
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NAME\tCLUSTER\tUSER\tNAMESPACE")
	for _, name := range m.detail.file.ContextNames() {
		ctx := f.Config.Contexts[name]
		ns := ctx.Namespace
		if ns == "" {
			ns = "-"
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", name, ctx.Cluster, ctx.AuthInfo, ns)
	}
	_ = tw.Flush()

	b.WriteString("\n")
	b.WriteString(m.renderFooter("esc back  x select  q quit"))
	return b.String()
}

func (m Model) viewTagEdit() string {
	fi, ok := m.currentItem()
	title := "Edit tags"
	if ok {
		title = "Tags for " + fi.file.Name()
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		detailHeaderStyle.Render(title),
		"",
		"Comma-separated tags:",
		m.tagInput.View(),
		"",
		helpStyle.Render("enter save  esc cancel"),
	)
	return modalBorderStyle.Render(body)
}

func (m Model) viewRename() string {
	fi, ok := m.currentItem()
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

func toggleAlert(store state.Store, hash, pathHint string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.Entries[hash]
		entry.PathHint = pathHint
		entry.Alerts.Enabled = !entry.Alerts.Enabled
		if entry.Alerts.Enabled {
			if !entry.Alerts.RequireConfirmation && !entry.Alerts.ConfirmClusterName {
				entry.Alerts.RequireConfirmation = true
			}
			if len(entry.Alerts.BlockedVerbs) == 0 {
				entry.Alerts.BlockedVerbs = state.DefaultBlockedVerbs()
			}
		}
		entry.Touch()
		cfg.Entries[hash] = entry
		return nil
	})
}

func setTags(store state.Store, hash, pathHint string, tags []string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.Entries[hash]
		entry.PathHint = pathHint
		entry.Tags = tags
		entry.Touch()
		cfg.Entries[hash] = entry
		return nil
	})
}

func rebindPathHint(store state.Store, hash, newHint string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry, ok := cfg.Entries[hash]
		if !ok {
			return nil
		}
		entry.PathHint = newHint
		entry.Touch()
		cfg.Entries[hash] = entry
		return nil
	})
}
