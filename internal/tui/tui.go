// Package tui implements the interactive kubeconfig-manager TUI.
//
// The package is organized by screen / mode — this file owns the shared
// Model, the bubbletea Run/Init/Update/View entry points, and the chrome
// (header/footer). Each mode lives in its own file with its update/view pair
// and any mode-local state: list_view.go, detail_view.go, palette_view.go,
// tag_editor.go, rename_view.go, ctx_ops.go, import_merge.go. On-disk
// mutations and the reload command are in mutations.go; styling is in
// styles.go; the palette-backed tag picker widget is in tag_picker.go.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/loupeznik/kubeconfig-manager/internal/state"
)

type mode int

const (
	modeList mode = iota
	modeDetail
	modeTagEdit
	modeRename
	modePalette
	modeCtxRename
	modeCtxDelete
	modeCtxSplit
	modeImport
	modeMergeSource
	modeMergeOutput
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

// Model holds the full TUI state. Fields are grouped by responsibility so the
// mode-specific files can reason about what they touch. Kept as a single
// struct (rather than per-mode sub-models) because bubbletea's Update/View
// contract threads one Model through the program.
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

	ctxInput      textinput.Model
	ctxActionName string // context name pending rename/delete/split

	fileInput    textinput.Model // shared for import/merge flows
	mergeSourceB string          // cached path from modeMergeSource into modeMergeOutput

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
	// Point lipgloss's default renderer at stderr BEFORE any bubbles component
	// is constructed. bubbles/textinput and bubbles/cursor create their styles
	// via lipgloss.NewStyle() which binds to whatever the default renderer is
	// at call time. Without this, the shell-hook flow (stdout captured by
	// `eval "$(...)"`) leaves the cursor's Reverse attribute stripped because
	// stdout is classified as no-color — making the caret invisible.
	lipgloss.SetDefaultRenderer(renderer)

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

	ci := textinput.New()
	ci.Prompt = "> "
	ci.CharLimit = 255

	fi := textinput.New()
	fi.Prompt = "> "
	fi.CharLimit = 512

	return Model{
		mode:         modeList,
		dir:          dir,
		version:      version,
		store:        store,
		fileList:     l,
		ctxList:      newStyledList("context", "contexts", nil, ctxListKeyBindings),
		paletteList:  newStyledList("tag", "tags", nil, paletteListKeyBindings),
		paletteInput: pi,
		ctxInput:     ci,
		fileInput:    fi,
		tagInput:     ti,
		renameInput:  ri,
	}, nil
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
		case modeCtxRename:
			return m.updateCtxRename(msg)
		case modeCtxDelete:
			return m.updateCtxDelete(msg)
		case modeCtxSplit:
			return m.updateCtxSplit(msg)
		case modeImport:
			return m.updateImport(msg)
		case modeMergeSource:
			return m.updateMergeSource(msg)
		case modeMergeOutput:
			return m.updateMergeOutput(msg)
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
			m.detailFile = refindFile(items, m.detailFile.path)
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
	case modeCtxRename, modeCtxSplit:
		m.ctxInput, cmd = m.ctxInput.Update(msg)
	case modeImport, modeMergeSource, modeMergeOutput:
		m.fileInput, cmd = m.fileInput.Update(msg)
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
	case modeCtxRename:
		return m.viewCtxRename()
	case modeCtxDelete:
		return m.viewCtxDelete()
	case modeCtxSplit:
		return m.viewCtxSplit()
	case modeImport:
		return m.viewImport()
	case modeMergeSource:
		return m.viewMergeSource()
	case modeMergeOutput:
		return m.viewMergeOutput()
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
			renderKey("i", "import"),
			renderKey("m", "merge"),
			renderKey("p", "palette"),
			renderKey("/", "filter"),
			renderKey("q", "quit"),
		}, separatorStyle.Render(" · "))
	case modeDetail:
		keys = strings.Join([]string{
			renderKey("t", "tags"),
			renderKey("a", "toggle alerts"),
			renderKey("R", "rename ctx"),
			renderKey("D", "delete ctx"),
			renderKey("S", "split ctx"),
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
