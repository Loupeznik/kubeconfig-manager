package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

const (
	chromeHeight = 3 // title line + spacer + footer line
)

type Model struct {
	mode        mode
	dir         string
	version     string
	store       state.Store
	fileList    list.Model
	ctxList     list.Model
	tagInput    textinput.Model
	renameInput textinput.Model

	tagPicker     *tagPicker // non-nil when palette is populated
	detailFile    *fileItem  // the file whose contexts are shown in ctxList
	targetContext string     // when non-empty, tag/alert actions apply per-context

	width  int
	height int
	status string
	stErr  bool

	selectedPath string
}

func Run(ctx context.Context, dir, version string, store state.Store) (string, error) {
	m, err := newModel(ctx, dir, version, store)
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

func newModel(ctx context.Context, dir, version string, store state.Store) (Model, error) {
	items, err := loadFileItems(ctx, dir, store)
	if err != nil {
		return Model{}, err
	}

	l := newStyledList("kubeconfigs", items, listKeyBindings)
	c := newStyledList("contexts", nil, detailKeyBindings)

	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 512

	ri := textinput.New()
	ri.Prompt = "> "
	ri.CharLimit = 255

	return Model{
		mode:        modeList,
		dir:         dir,
		version:     version,
		store:       store,
		fileList:    l,
		ctxList:     c,
		tagInput:    ti,
		renameInput: ri,
	}, nil
}

func newStyledList(label string, items []list.Item, extraKeys func() []key.Binding) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(colorAccent)).
		BorderForeground(lipgloss.Color(colorAccent))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(colorLavender)).
		BorderForeground(lipgloss.Color(colorAccent))
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(lipgloss.Color(colorText))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(lipgloss.Color(colorMuted))

	l := list.New(items, delegate, 0, 0)
	l.Title = label
	l.Styles.Title = listTitleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName(label, label+"s")
	l.AdditionalShortHelpKeys = extraKeys
	l.AdditionalFullHelpKeys = extraKeys
	return l
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		innerHeight := msg.Height - chromeHeight
		if innerHeight < 5 {
			innerHeight = 5
		}
		m.fileList.SetSize(msg.Width, innerHeight)
		m.ctxList.SetSize(msg.Width, innerHeight)
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
		items, err := loadFileItems(context.Background(), m.dir, m.store)
		if err != nil {
			m.setErr(fmt.Sprintf("reload failed: %v", err))
			return m, nil
		}
		m.fileList.SetItems(items)
		if m.detailFile != nil {
			m.detailFile = refindFile(items, m.detailFile.hash)
			if m.detailFile != nil {
				cfg, cerr := m.store.Load(context.Background())
				if cerr == nil {
					m.ctxList.SetItems(buildContextItems(m.detailFile, cfg.Entries[m.detailFile.hash]))
				}
			}
		}
		if msg.status != "" {
			m.setStatus(msg.status)
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.mode {
	case modeList:
		m.fileList, cmd = m.fileList.Update(msg)
	case modeDetail:
		m.ctxList, cmd = m.ctxList.Update(msg)
	case modeTagEdit:
		m.tagInput, cmd = m.tagInput.Update(msg)
	case modeRename:
		m.renameInput, cmd = m.renameInput.Update(msg)
	}
	return m, cmd
}

func (m Model) View() string {
	body := ""
	switch m.mode {
	case modeList:
		body = m.fileList.View()
	case modeDetail:
		body = m.viewDetail()
	case modeTagEdit:
		return m.viewTagEdit()
	case modeRename:
		return m.viewRename()
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		body,
		m.renderFooter(),
	)
}

func (m Model) renderHeader() string {
	title := appTitleStyle.Render("kubeconfig-manager")
	ver := ""
	if m.version != "" {
		ver = versionStyle.Render("v" + strings.TrimPrefix(m.version, "v"))
	}
	dir := dirHintStyle.Render("dir: " + m.dir)
	return lipgloss.JoinHorizontal(lipgloss.Top, title, ver, dir)
}

func (m Model) renderFooter() string {
	var keys string
	switch m.mode {
	case modeList:
		keys = strings.Join([]string{
			renderKey("s", "select"),
			renderKey("↵", "details"),
			renderKey("t", "tags"),
			renderKey("a", "alerts"),
			renderKey("r", "rename"),
			renderKey("/", "filter"),
			renderKey("q", "quit"),
		}, separatorStyle.Render(" · "))
	case modeDetail:
		keys = strings.Join([]string{
			renderKey("t", "tags"),
			renderKey("a", "toggle alerts"),
			renderKey("esc", "back"),
			renderKey("q", "quit"),
		}, separatorStyle.Render(" · "))
	}
	status := ""
	if m.status != "" {
		st := statusStyle
		if m.stErr {
			st = errorStyle
		}
		status = st.Render(m.status)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, keys, status)
}

func (m *Model) setStatus(s string) { m.status = s; m.stErr = false }
func (m *Model) setErr(s string)    { m.status = s; m.stErr = true }

// ============================================================================
// List view
// ============================================================================

type fileItem struct {
	path  string
	file  *kubeconfig.File
	entry state.Entry
	hash  string
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
		items := buildContextItems(&fi, fi.entry)
		m.ctxList.SetItems(items)
		m.detailFile = &fi
		m.mode = modeDetail
		return m, nil
	case "t":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		m.targetContext = ""
		m.openTagEditor(fi.entry.Tags)
		return m, nil
	case "r":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		m.renameInput.SetValue(fi.file.Name())
		m.renameInput.Focus()
		m.mode = modeRename
		return m, nil
	case "a":
		fi, ok := m.currentFileItem()
		if !ok {
			return m, nil
		}
		wasEnabled := fi.entry.Alerts.Enabled
		if err := toggleAlert(m.store, fi.hash, filepath.Base(fi.path), ""); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		status := "alerts enabled (file-level)"
		if wasEnabled {
			status = "alerts disabled (file-level)"
		}
		return m, reloadCmd(status)
	}

	var cmd tea.Cmd
	m.fileList, cmd = m.fileList.Update(msg)
	return m, cmd
}

// ============================================================================
// Detail view (interactive context list)
// ============================================================================

type contextItem struct {
	name       string
	cluster    string
	user       string
	namespace  string
	isCurrent  bool
	fileTags   []string
	ctxTags    []string
	ctxAlerts  state.Alerts
	fileAlerts state.Alerts
}

func (i contextItem) Title() string {
	if i.isCurrent {
		return currentContextStyle.Render("→ ") + i.name
	}
	return "  " + i.name
}

func (i contextItem) Description() string {
	var parts []string
	parts = append(parts, detailLabelStyle.Render("cluster:")+" "+detailValueStyle.Render(i.cluster))
	parts = append(parts, detailLabelStyle.Render("user:")+" "+detailValueStyle.Render(i.user))
	if i.namespace != "" {
		parts = append(parts, detailLabelStyle.Render("ns:")+" "+detailValueStyle.Render(i.namespace))
	}

	effectiveTags := mergeTags(i.fileTags, i.ctxTags)
	if len(effectiveTags) > 0 {
		parts = append(parts, contextTagStyle.Render(renderTagBadges(effectiveTags)))
	}

	// Alert marker: context-level wins over file-level.
	if i.ctxAlerts.Enabled {
		parts = append(parts, alertBadgeStyle.Render("⚠ ctx alerts"))
	} else if i.fileAlerts.Enabled && !isContextAlertsExplicitlyDisabled(i.ctxAlerts) {
		parts = append(parts, alertBadgeStyle.Render("⚠ file alerts"))
	}
	return strings.Join(parts, "  ")
}

func (i contextItem) FilterValue() string {
	return i.name + " " + strings.Join(i.ctxTags, " ")
}

func isContextAlertsExplicitlyDisabled(a state.Alerts) bool {
	// The zero Alerts is "no override". Only treat as explicit disable if
	// any field diverges from zero AND Enabled is false.
	return !a.Enabled && (a.RequireConfirmation || a.ConfirmClusterName || len(a.BlockedVerbs) > 0)
}

func mergeTags(file, ctx []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range file {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range ctx {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func detailKeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alerts")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func (m Model) currentContextItem() (contextItem, bool) {
	if m.ctxList.SelectedItem() == nil {
		return contextItem{}, false
	}
	ci, ok := m.ctxList.SelectedItem().(contextItem)
	return ci, ok
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ctxList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.ctxList, cmd = m.ctxList.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeList
		m.detailFile = nil
		m.targetContext = ""
		return m, nil
	case "t":
		ci, ok := m.currentContextItem()
		if !ok || m.detailFile == nil {
			return m, nil
		}
		m.targetContext = ci.name
		m.openTagEditor(ci.ctxTags)
		return m, nil
	case "a":
		ci, ok := m.currentContextItem()
		if !ok || m.detailFile == nil {
			return m, nil
		}
		entry := m.detailFile.entry
		wasEnabled := entry.ResolveAlerts(ci.name).Enabled
		if err := toggleAlert(m.store, m.detailFile.hash, filepath.Base(m.detailFile.path), ci.name); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		status := fmt.Sprintf("alerts enabled for context %s", ci.name)
		if wasEnabled {
			status = fmt.Sprintf("alerts disabled for context %s", ci.name)
		}
		return m, reloadCmd(status)
	}

	var cmd tea.Cmd
	m.ctxList, cmd = m.ctxList.Update(msg)
	return m, cmd
}

func (m Model) viewDetail() string {
	if m.detailFile == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render("(no selection)")
	}
	header := detailHeaderStyle.Render(m.detailFile.file.Name())
	if c := m.detailFile.file.Config.CurrentContext; c != "" {
		header += "  " + currentContextStyle.Render("current: "+c)
	}
	if len(m.detailFile.entry.Tags) > 0 {
		header += "  " + tagStyle.Render(renderTagBadges(m.detailFile.entry.Tags))
	}
	if m.detailFile.entry.Alerts.Enabled {
		header += "  " + alertBadgeStyle.Render("⚠ FILE ALERTS")
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, m.ctxList.View())
}

// ============================================================================
// Tag edit view
// ============================================================================

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
	if err := setTags(m.store, fi.hash, filepath.Base(fi.path), m.targetContext, newTags); err != nil {
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
// textinput fallback depending on whether the palette has any tags.
func (m *Model) openTagEditor(currentTags []string) {
	cfg, err := m.store.Load(context.Background())
	if err == nil {
		cfg.EnsurePaletteFromEntries()
		if len(cfg.AvailableTags) > 0 {
			m.tagPicker = newTagPicker(cfg.AvailableTags, currentTags)
			m.mode = modeTagEdit
			return
		}
	}
	m.tagPicker = nil
	m.tagInput.SetValue(strings.Join(currentTags, ", "))
	m.tagInput.Focus()
	m.mode = modeTagEdit
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

// ============================================================================
// Rename view
// ============================================================================

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

// ============================================================================
// Load / mutate helpers + reload message
// ============================================================================

type reloadMsg struct {
	status string
}

func reloadCmd(status string) tea.Cmd {
	return func() tea.Msg { return reloadMsg{status: status} }
}

func loadFileItems(ctx context.Context, dir string, store state.Store) ([]list.Item, error) {
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

func refindFile(items []list.Item, hash string) *fileItem {
	for _, it := range items {
		fi, ok := it.(fileItem)
		if ok && fi.hash == hash {
			copy := fi
			return &copy
		}
	}
	return nil
}

func buildContextItems(fi *fileItem, entry state.Entry) []list.Item {
	names := make([]string, 0, len(fi.file.Config.Contexts))
	for n := range fi.file.Config.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]list.Item, 0, len(names))
	for _, n := range names {
		ctx := fi.file.Config.Contexts[n]
		var cluster, user, ns string
		if ctx != nil {
			cluster = ctx.Cluster
			user = ctx.AuthInfo
			ns = ctx.Namespace
		}
		var ctxTags []string
		if entry.ContextTags != nil {
			ctxTags = entry.ContextTags[n]
		}
		var ctxAlerts state.Alerts
		if entry.ContextAlerts != nil {
			ctxAlerts = entry.ContextAlerts[n]
		}
		out = append(out, contextItem{
			name:       n,
			cluster:    cluster,
			user:       user,
			namespace:  ns,
			isCurrent:  n == fi.file.Config.CurrentContext,
			fileTags:   entry.Tags,
			ctxTags:    ctxTags,
			ctxAlerts:  ctxAlerts,
			fileAlerts: entry.Alerts,
		})
	}
	return out
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

// toggleAlert flips alerts for the file (contextName == "") or for a specific
// context within the file.
func toggleAlert(store state.Store, hash, pathHint, contextName string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.Entries[hash]
		entry.PathHint = pathHint
		if contextName == "" {
			entry.Alerts.Enabled = !entry.Alerts.Enabled
			if entry.Alerts.Enabled {
				if !entry.Alerts.RequireConfirmation && !entry.Alerts.ConfirmClusterName {
					entry.Alerts.RequireConfirmation = true
				}
				if len(entry.Alerts.BlockedVerbs) == 0 {
					entry.Alerts.BlockedVerbs = state.DefaultBlockedVerbs()
				}
			}
		} else {
			if entry.ContextAlerts == nil {
				entry.ContextAlerts = map[string]state.Alerts{}
			}
			a := entry.ContextAlerts[contextName]
			a.Enabled = !a.Enabled
			if a.Enabled {
				if !a.RequireConfirmation && !a.ConfirmClusterName {
					a.RequireConfirmation = true
				}
				if len(a.BlockedVerbs) == 0 {
					a.BlockedVerbs = state.DefaultBlockedVerbs()
				}
			}
			entry.ContextAlerts[contextName] = a
		}
		entry.Touch()
		cfg.Entries[hash] = entry
		return nil
	})
}

// setTags replaces tags at the file level (contextName == "") or for a specific
// context within the file.
func setTags(store state.Store, hash, pathHint, contextName string, tags []string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.Entries[hash]
		entry.PathHint = pathHint
		if contextName == "" {
			entry.Tags = tags
		} else {
			if entry.ContextTags == nil {
				entry.ContextTags = map[string][]string{}
			}
			if len(tags) == 0 {
				delete(entry.ContextTags, contextName)
			} else {
				entry.ContextTags[contextName] = tags
			}
		}
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
