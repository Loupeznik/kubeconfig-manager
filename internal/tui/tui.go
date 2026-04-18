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
	modePalette
)

type paletteAction int

const (
	paletteBrowsing paletteAction = iota
	paletteAdding
	paletteRenaming
	paletteDeleting
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

	paletteList       list.Model
	paletteInput      textinput.Model
	paletteAction     paletteAction
	paletteDelete     string
	paletteRenameFrom string
	paletteUsage      map[string][]string

	tagPicker     *tagPicker // non-nil when palette is populated
	detailFile    *fileItem  // the file whose contexts are shown in ctxTable
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

	l := newStyledList("kubeconfig", "kubeconfigs", items, listKeyBindings)

	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 512

	ri := textinput.New()
	ri.Prompt = "> "
	ri.CharLimit = 255

	pi := textinput.New()
	pi.Prompt = "> "
	pi.CharLimit = 64
	pi.Placeholder = "new tag name"

	return Model{
		mode:         modeList,
		dir:          dir,
		version:      version,
		store:        store,
		fileList:     l,
		ctxList:      newStyledList("context", "contexts", nil, ctxListKeyBindings),
		paletteList:  newStyledList("tag", "tags", nil, paletteListKeyBindings),
		paletteInput: pi,
		tagInput:     ti,
		renameInput:  ri,
	}, nil
}

func paletteListKeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	}
}

func ctxListKeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags")),
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alerts")),
	}
}

func newStyledList(singular, plural string, items []list.Item, extraKeys func() []key.Binding) list.Model {
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
	l.Title = plural
	l.Styles.Title = listTitleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName(singular, plural)
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
		// leave room for the detail header line
		ctxHeight := innerHeight - 2
		if ctxHeight < 5 {
			ctxHeight = 5
		}
		m.ctxList.SetSize(msg.Width, ctxHeight)
		m.paletteList.SetSize(msg.Width, innerHeight)
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
		case modePalette:
			return m.updatePalette(msg)
		}

	case paletteReloadMsg:
		cfg, err := m.store.Load(context.Background())
		if err != nil {
			m.setErr("reload palette: " + err.Error())
			return m, nil
		}
		cfg.EnsurePaletteFromEntries()
		m.loadPaletteList(cfg)
		if msg.status != "" {
			m.setStatus(msg.status)
		}
		return m, nil

	case reloadMsg:
		items, err := loadFileItems(context.Background(), m.dir, m.store)
		if err != nil {
			m.setErr(fmt.Sprintf("reload failed: %v", err))
			return m, nil
		}
		m.fileList.SetItems(items)
		if m.detailFile != nil {
			m.detailFile = refindFile(items, m.detailFile.identity.StableHash)
			if m.detailFile != nil {
				cfg, cerr := m.store.Load(context.Background())
				if cerr == nil {
					entry, _ := cfg.GetEntry(m.detailFile.identity.StableHash, m.detailFile.identity.ContentHash)
					idx := m.ctxList.Index()
					m.loadContextList(m.detailFile, entry)
					if idx < len(m.ctxList.Items()) {
						m.ctxList.Select(idx)
					}
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
	case modePalette:
		if m.paletteAction == paletteAdding || m.paletteAction == paletteRenaming {
			m.paletteInput, cmd = m.paletteInput.Update(msg)
		} else {
			m.paletteList, cmd = m.paletteList.Update(msg)
		}
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
	case modePalette:
		body = m.viewPalette()
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
			renderKey("p", "palette"),
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
	case modePalette:
		switch m.paletteAction {
		case paletteAdding:
			keys = strings.Join([]string{
				renderKey("↵", "add"),
				renderKey("esc", "cancel"),
			}, separatorStyle.Render(" · "))
		case paletteRenaming:
			keys = strings.Join([]string{
				renderKey("↵", "save"),
				renderKey("esc", "cancel"),
			}, separatorStyle.Render(" · "))
		case paletteDeleting:
			keys = strings.Join([]string{
				renderKey("y", "confirm delete"),
				renderKey("n/esc", "cancel"),
			}, separatorStyle.Render(" · "))
		default:
			keys = strings.Join([]string{
				renderKey("n", "new"),
				renderKey("r", "rename"),
				renderKey("d", "delete"),
				renderKey("/", "filter"),
				renderKey("esc", "back"),
				renderKey("q", "quit"),
			}, separatorStyle.Render(" · "))
		}
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
	}

	var cmd tea.Cmd
	m.fileList, cmd = m.fileList.Update(msg)
	return m, cmd
}

// ============================================================================
// Palette management view
// ============================================================================

func (m *Model) openPalette() {
	// Persist the auto-bootstrap so any subsequent mutation (add/rename/delete)
	// operates on the fully materialized palette rather than an empty one.
	if err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
		cfg.EnsurePaletteFromEntries()
		return nil
	}); err != nil {
		m.setErr("bootstrap palette: " + err.Error())
		return
	}
	cfg, err := m.store.Load(context.Background())
	if err != nil {
		m.setErr("load state: " + err.Error())
		return
	}
	m.loadPaletteList(cfg)
	m.paletteAction = paletteBrowsing
	m.paletteDelete = ""
	m.paletteRenameFrom = ""
	m.paletteInput.SetValue("")
	m.paletteInput.Blur()
	// Size the palette list; the outer Update will re-size on next window event,
	// but do a sensible fallback so the list shows something immediately.
	if m.width > 0 && m.height > chromeHeight {
		m.paletteList.SetSize(m.width, m.height-chromeHeight)
	}
	m.mode = modePalette
}

// paletteItem is a single tag row in the palette list.
type paletteItem struct {
	tag       string
	locations []string // file / file-context locations where it's used
}

func (p paletteItem) Title() string {
	return tagStyle.Render("●") + " " + p.tag
}

func (p paletteItem) Description() string {
	if len(p.locations) == 0 {
		return detailLabelStyle.Render("unused")
	}
	used := strings.Join(p.locations, ", ")
	if len(used) > 80 {
		used = used[:77] + "..."
	}
	return detailLabelStyle.Render(fmt.Sprintf("used in %d place(s): ", len(p.locations))) + detailValueStyle.Render(used)
}

func (p paletteItem) FilterValue() string { return p.tag }

func (m *Model) loadPaletteList(cfg *state.Config) {
	usage := tagUsage(cfg)
	m.paletteUsage = usage

	items := make([]list.Item, 0, len(cfg.AvailableTags))
	for _, t := range cfg.AvailableTags {
		items = append(items, paletteItem{
			tag:       t,
			locations: usage[t],
		})
	}
	prevIndex := m.paletteList.Index()
	m.paletteList.SetItems(items)
	if prevIndex < len(items) {
		m.paletteList.Select(prevIndex)
	}
}

// tagUsage returns, for each tag, a sorted list of "file" or "file/context"
// locations where it is referenced.
func tagUsage(cfg *state.Config) map[string][]string {
	out := map[string][]string{}
	for _, entry := range cfg.Entries {
		label := entry.PathHint
		if label == "" {
			label = "—"
		}
		for _, t := range entry.Tags {
			out[t] = appendUnique(out[t], label)
		}
		for ctxName, ctxTags := range entry.ContextTags {
			for _, t := range ctxTags {
				out[t] = appendUnique(out[t], label+"/"+ctxName)
			}
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

func appendUnique(xs []string, s string) []string {
	for _, existing := range xs {
		if existing == s {
			return xs
		}
	}
	return append(xs, s)
}

func (m Model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.paletteAction {
	case paletteAdding:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.paletteAction = paletteBrowsing
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			return m, nil
		case "enter":
			tag := strings.TrimSpace(m.paletteInput.Value())
			m.paletteAction = paletteBrowsing
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			if tag == "" {
				return m, nil
			}
			var added []string
			err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
				cfg.EnsurePaletteFromEntries()
				added = cfg.AddAvailableTags(tag)
				return nil
			})
			if err != nil {
				m.setErr("add tag: " + err.Error())
				return m, nil
			}
			msg := "added to palette: " + tag
			if len(added) == 0 {
				msg = tag + " already in palette"
			}
			return m, m.paletteReload(msg)
		}
		var cmd tea.Cmd
		m.paletteInput, cmd = m.paletteInput.Update(msg)
		return m, cmd

	case paletteRenaming:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.paletteAction = paletteBrowsing
			m.paletteRenameFrom = ""
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			return m, nil
		case "enter":
			oldTag := m.paletteRenameFrom
			newTag := strings.TrimSpace(m.paletteInput.Value())
			m.paletteAction = paletteBrowsing
			m.paletteRenameFrom = ""
			m.paletteInput.SetValue("")
			m.paletteInput.Blur()
			if newTag == "" || newTag == oldTag {
				return m, nil
			}
			err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
				cfg.EnsurePaletteFromEntries()
				return cfg.RenameAvailableTag(oldTag, newTag)
			})
			if err != nil {
				m.setErr("rename tag: " + err.Error())
				return m, nil
			}
			return m, tea.Batch(m.paletteReload(fmt.Sprintf("renamed %s → %s", oldTag, newTag)), reloadCmd(""))
		}
		var cmd tea.Cmd
		m.paletteInput, cmd = m.paletteInput.Update(msg)
		return m, cmd

	case paletteDeleting:
		switch msg.String() {
		case "y":
			victim := m.paletteDelete
			m.paletteAction = paletteBrowsing
			m.paletteDelete = ""
			if victim == "" {
				return m, nil
			}
			var removed []string
			err := m.store.Mutate(context.Background(), func(cfg *state.Config) error {
				cfg.EnsurePaletteFromEntries()
				removed = cfg.RemoveAvailableTags(victim)
				return nil
			})
			if err != nil {
				m.setErr("delete tag: " + err.Error())
				return m, nil
			}
			if len(removed) == 0 {
				return m, m.paletteReload(victim + " was not in palette")
			}
			return m, tea.Batch(m.paletteReload("deleted and scrubbed: "+victim), reloadCmd(""))
		case "n", "esc", "ctrl+c":
			m.paletteAction = paletteBrowsing
			m.paletteDelete = ""
			return m, nil
		}
		return m, nil

	default: // paletteBrowsing
		if m.paletteList.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.paletteList, cmd = m.paletteList.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.mode = modeList
			m.paletteAction = paletteBrowsing
			m.paletteDelete = ""
			m.paletteRenameFrom = ""
			return m, nil
		case "n":
			m.paletteAction = paletteAdding
			m.paletteInput.SetValue("")
			m.paletteInput.Focus()
			return m, nil
		case "r":
			tag := m.currentPaletteTag()
			if tag == "" {
				return m, nil
			}
			m.paletteAction = paletteRenaming
			m.paletteRenameFrom = tag
			m.paletteInput.SetValue(tag)
			m.paletteInput.Focus()
			return m, nil
		case "d":
			tag := m.currentPaletteTag()
			if tag == "" {
				return m, nil
			}
			m.paletteAction = paletteDeleting
			m.paletteDelete = tag
			return m, nil
		}
		var cmd tea.Cmd
		m.paletteList, cmd = m.paletteList.Update(msg)
		return m, cmd
	}
}

func (m Model) currentPaletteTag() string {
	if m.paletteList.SelectedItem() == nil {
		return ""
	}
	pi, ok := m.paletteList.SelectedItem().(paletteItem)
	if !ok {
		return ""
	}
	return pi.tag
}

func (m Model) paletteReload(status string) tea.Cmd {
	return func() tea.Msg { return paletteReloadMsg{status: status} }
}

type paletteReloadMsg struct {
	status string
}

func (m Model) viewPalette() string {
	title := detailHeaderStyle.Render("Tag palette")
	header := title

	body := m.paletteList.View()

	switch m.paletteAction {
	case paletteAdding:
		body += "\n" + modalBorderStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			detailLabelStyle.Render("New tag name:"),
			m.paletteInput.View(),
		))
	case paletteRenaming:
		body += "\n" + modalBorderStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			detailLabelStyle.Render("Rename "+m.paletteRenameFrom+" to:"),
			m.paletteInput.View(),
			"",
			dirHintStyle.Render("All usages across kubeconfigs and contexts will be updated."),
		))
	case paletteDeleting:
		body += "\n" + modalBorderStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
			detailHeaderStyle.Render("Delete tag "+alertBadgeStyle.Render(m.paletteDelete)+"?"),
			detailValueStyle.Render("This also removes it from every kubeconfig and context that uses it."),
		))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

// ============================================================================
// Detail view (interactive context table)
// ============================================================================

// contextRow is the in-memory representation of a row in the detail table.
// Kept aligned with the table's Rows slice so keybindings can look up the
// per-context data by cursor index.
type contextRow struct {
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

func (r contextRow) effectiveTags() []string { return mergeTags(r.fileTags, r.ctxTags) }

// alertIndicator returns the badge for the effective alert state, or "" when
// there's nothing meaningful to show. "alerts off (override)" only appears
// when the context explicitly suppresses a file-level policy that would
// otherwise apply — showing it when there's no file-level policy in the first
// place would be confusing noise.
func (r contextRow) alertIndicator() string {
	if r.ctxAlerts.Enabled {
		return alertBadgeStyle.Render("⚠ ctx alerts")
	}
	if !r.fileAlerts.Enabled {
		return ""
	}
	if isContextAlertsExplicitlyDisabled(r.ctxAlerts) {
		return detailLabelStyle.Render("alerts off (override)")
	}
	return alertBadgeStyle.Render("⚠ file alerts")
}

func isContextAlertsExplicitlyDisabled(a state.Alerts) bool {
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

// ContextItem is a single context rendered in the detail list. Matches the
// main file list's title/description layout for visual consistency.
type contextItem struct {
	row contextRow
}

func (i contextItem) Title() string {
	if i.row.isCurrent {
		return currentContextStyle.Render("●") + " " + i.row.name
	}
	return "  " + i.row.name
}

func (i contextItem) Description() string {
	sep := separatorStyle.Render(" · ")
	var parts []string
	if i.row.cluster != "" {
		parts = append(parts, detailValueStyle.Render(i.row.cluster))
	}
	if i.row.user != "" {
		parts = append(parts, detailValueStyle.Render(i.row.user))
	}
	if i.row.namespace != "" {
		parts = append(parts, detailValueStyle.Render(i.row.namespace))
	}
	if eff := i.row.effectiveTags(); len(eff) > 0 {
		parts = append(parts, tagStyle.Render(renderTagBadges(eff)))
	}
	if badge := i.row.alertIndicator(); badge != "" {
		parts = append(parts, badge)
	}
	return strings.Join(parts, sep)
}

func (i contextItem) FilterValue() string {
	return i.row.name + " " + strings.Join(i.row.ctxTags, " ")
}

func (m *Model) loadContextList(fi *fileItem, entry state.Entry) {
	rows := buildContextRows(fi, entry)
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, contextItem{row: r})
	}
	m.ctxList.SetItems(items)
}

func (m Model) currentContextRow() (contextRow, bool) {
	if m.ctxList.SelectedItem() == nil {
		return contextRow{}, false
	}
	ci, ok := m.ctxList.SelectedItem().(contextItem)
	if !ok {
		return contextRow{}, false
	}
	return ci.row, true
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Let the list handle keys while actively filtering.
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
		r, ok := m.currentContextRow()
		if !ok || m.detailFile == nil {
			return m, nil
		}
		m.targetContext = r.name
		m.openTagEditor(r.ctxTags)
		return m, nil
	case "a":
		r, ok := m.currentContextRow()
		if !ok || m.detailFile == nil {
			return m, nil
		}
		entry := m.detailFile.entry
		wasEnabled := entry.ResolveAlerts(r.name).Enabled
		if err := toggleAlert(m.store, m.detailFile.identity, filepath.Base(m.detailFile.path), r.name); err != nil {
			m.setErr(err.Error())
			return m, nil
		}
		status := fmt.Sprintf("alerts enabled for context %s", r.name)
		if wasEnabled {
			status = fmt.Sprintf("alerts disabled for context %s", r.name)
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

	var headerParts []string
	headerParts = append(headerParts, detailHeaderStyle.Render(m.detailFile.file.Name()))
	if c := m.detailFile.file.Config.CurrentContext; c != "" {
		headerParts = append(headerParts,
			detailLabelStyle.Render("current: ")+currentContextStyle.Render(c))
	}
	if len(m.detailFile.entry.Tags) > 0 {
		headerParts = append(headerParts, tagStyle.Render(renderTagBadges(m.detailFile.entry.Tags)))
	}
	if m.detailFile.entry.Alerts.Enabled {
		headerParts = append(headerParts, alertBadgeStyle.Render("⚠ FILE ALERTS"))
	}
	header := strings.Join(headerParts, separatorStyle.Render(" · "))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.ctxList.View(),
	)
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
		id, err := kubeconfig.IdentifyFile(f.Path)
		if err != nil {
			return nil, err
		}
		entry, _ := cfg.GetEntry(id.StableHash, id.ContentHash)
		items = append(items, fileItem{
			path:     f.Path,
			file:     f,
			entry:    entry,
			identity: id,
		})
	}
	return items, nil
}

func refindFile(items []list.Item, stableHash string) *fileItem {
	for _, it := range items {
		fi, ok := it.(fileItem)
		if ok && fi.identity.StableHash == stableHash {
			copy := fi
			return &copy
		}
	}
	return nil
}

func buildContextRows(fi *fileItem, entry state.Entry) []contextRow {
	names := make([]string, 0, len(fi.file.Config.Contexts))
	for n := range fi.file.Config.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]contextRow, 0, len(names))
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
		out = append(out, contextRow{
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
func toggleAlert(store state.Store, id kubeconfig.Identity, pathHint, contextName string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.TakeEntry(id.StableHash, id.ContentHash)
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
		cfg.Entries[id.StableHash] = entry
		return nil
	})
}

// setTags replaces tags at the file level (contextName == "") or for a specific
// context within the file.
func setTags(store state.Store, id kubeconfig.Identity, pathHint, contextName string, tags []string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry := cfg.TakeEntry(id.StableHash, id.ContentHash)
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
		cfg.Entries[id.StableHash] = entry
		return nil
	})
}

func rebindPathHint(store state.Store, id kubeconfig.Identity, newHint string) error {
	return store.Mutate(context.Background(), func(cfg *state.Config) error {
		entry, ok := cfg.GetEntry(id.StableHash, id.ContentHash)
		if !ok {
			return nil
		}
		entry = cfg.TakeEntry(id.StableHash, id.ContentHash)
		entry.PathHint = newHint
		entry.Touch()
		cfg.Entries[id.StableHash] = entry
		return nil
	})
}
