package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/reflow/wordwrap"
)

const (
	refreshInterval    = 5 * time.Second
	version            = "v0.2.1"
	panelGap           = 2 // Slightly smaller gap looks cleaner
	stackedPanelGap    = 1
	minContentHeight   = 5
	defaultHistoryDays = 3
	envHistoryDays     = "SLURM_DASHBOARD_HISTORY_DAYS"
	// CHANGED: Increased from 6 to 8.
	// This reserves more space for borders/padding so they don't get pushed out.
	panelChromeWidth = 8
	// CHANGED: Reduced to allow resizing on smaller screens
	minTablePanelWidth   = 30
	minDetailsPanelWidth = 20
	maxDetailsPanelWidth = 50
)

type mode int

const (
	modeLive mode = iota
	modeHistory
)

type statusFilter int

const (
	filterAll statusFilter = iota
	filterRunning
	filterPending
)

func (s statusFilter) String() string {
	switch s {
	case filterRunning:
		return "Running"
	case filterPending:
		return "Pending"
	default:
		return "All"
	}
}

// KeyMap defines the keybindings
type KeyMap struct {
	Quit         key.Binding
	CancelJob    key.Binding
	InspectJob   key.Binding
	TailLogs     key.Binding
	TailStdout   key.Binding // New
	TailStderr   key.Binding // New
	Filter       key.Binding
	Pause        key.Binding
	Refresh      key.Binding
	History      key.Binding
	StatusFilter key.Binding
	CopyValue    key.Binding
	ViewValue    key.Binding
	Up           key.Binding
	Down         key.Binding
	Enter        key.Binding
	SwitchFocus  key.Binding
	ToggleMouse  key.Binding
	ToggleHelp   key.Binding
}

var keys = KeyMap{
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	CancelJob:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel")),
	InspectJob:   key.NewBinding(key.WithKeys("i", "enter"), key.WithHelp("i/ent", "inspect")),
	TailLogs:     key.NewBinding(key.WithKeys("l", "L"), key.WithHelp("l", "tail logs")),
	TailStdout:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "stdout")),
	TailStderr:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "stderr")),
	Filter:       key.NewBinding(key.WithKeys("f", "/"), key.WithHelp("f", "filter")),
	Pause:        key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause")),
	Refresh:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	History:      key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "history")),
	StatusFilter: key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "status filter")),
	CopyValue:    key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^y", "copy detail")),
	ViewValue:    key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view value")),
	Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	SwitchFocus:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch focus")),
	ToggleMouse:  key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "toggle mouse")),
	ToggleHelp:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.Filter, k.Refresh, k.InspectJob, k.TailLogs, k.TailStdout, k.TailStderr, k.SwitchFocus, k.ToggleMouse, k.ToggleHelp}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.InspectJob, k.CancelJob},
		{k.Filter, k.StatusFilter, k.History, k.Refresh},
		{k.TailLogs, k.TailStdout, k.TailStderr, k.CopyValue, k.ViewValue, k.SwitchFocus, k.ToggleMouse, k.ToggleHelp, k.Pause, k.Quit},
	}
}

type tickMsg time.Time
type jobsMsg []Job
type detailsMsg string
type errMsg error
type refreshNowMsg struct{}
type tailPathsMsg struct {
	jobID          string
	stdout, stderr string
	mode           TailMode
	err            error
}

// Model is the main application model
type Model struct {
	table        table.Model
	detailsTable table.Model
	filterInput  textinput.Model
	help         help.Model

	tailModel  TailModel
	inTailView bool
	// Full-screen details view (useful when details panel is hidden on small windows).
	inDetailsOverlay bool
	// Full-screen single value view (for long detail values).
	inValueOverlay bool
	valueView      viewport.Model
	valueKey       string
	valueValue     string

	jobs       []Job
	filtered   []Job
	selectedID string

	fullColumns []table.Column // Store original columns for responsive layout

	// Confirmation state
	confirmingCancel bool
	cancelCandidate  *Job

	appMode     mode
	paused      bool
	sFilter     statusFilter
	loadingJobs bool
	historyDays int

	width  int
	height int

	tablePanelHeight    int
	detailsPanelHeight  int
	detailsContentWidth int
	tableBlockWidth     int
	detailsBlockWidth   int
	stackPanels         bool
	stackGapHeight      int
	hideDetails         bool

	lastRefresh time.Time
	err         error

	rawDetails   string // Store raw details for re-wrapping on resize
	inputMode    bool   // if true, focus on filter input
	mouseEnabled bool

	// Saved main-view mouse setting before entering tail view. Tail view may
	// auto-disable mouse for easier text selection/copying.
	mouseEnabledBeforeTail bool

	copyFeedback       string
	copyFeedbackExpiry time.Time
}

func NewModel() Model {
	// Table setup
	columns := []table.Column{
		{Title: "Job ID", Width: 8},
		{Title: "Name", Width: 16},
		{Title: "Status", Width: 10},
		{Title: "Time", Width: 10},
		{Title: "Nodes", Width: 6},
		{Title: "Partition", Width: 10},
		{Title: "Nodelist", Width: 15},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = tableHeaderStyle
	s.Selected = tableSelectedStyle
	t.SetStyles(s)

	// Details Table setup
	detailsCols := []table.Column{
		{Title: "Key", Width: 20},
		{Title: "Value", Width: 30},
	}
	dt := table.New(
		table.WithColumns(detailsCols),
		table.WithFocused(false),
		table.WithHeight(10),
	)

	dtStyles := table.DefaultStyles()
	dtStyles.Header = tableHeaderStyle
	dtStyles.Selected = tableSelectedStyle
	dt.SetStyles(dtStyles)

	// Input setup
	ti := textinput.New()
	ti.Placeholder = "Filter"
	ti.CharLimit = 50
	ti.Width = 20
	ti.Prompt = ""
	ti.PromptStyle = lipgloss.NewStyle().Foreground(subtle)
	ti.TextStyle = lipgloss.NewStyle().Foreground(textStrong)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(subtle)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(highlight)

	m := Model{
		table:        t,
		detailsTable: dt,
		filterInput:  ti,
		help:         help.New(),
		appMode:      modeLive,
		sFilter:      filterAll,
		fullColumns:  columns,
		mouseEnabled: false,
		historyDays:  historyDaysFromEnv(),
	}

	width, height := detectTerminalSize()
	m.applyWindowSize(width, height)

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchJobsCmd(),
		m.tickCmd(),
		tea.DisableMouse,
		initialWindowSizeCmd(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	handledTick := false

	if m.copyFeedback != "" && time.Now().After(m.copyFeedbackExpiry) {
		m.copyFeedback = ""
	}

	if _, ok := msg.(tickMsg); ok {
		handledTick = true
		// While tailing logs we still keep the tick loop alive so the app
		// continues to refresh normally after exiting, but we avoid polling
		// Slurm in the background.
		if !m.paused && !m.inTailView {
			cmds = append(cmds, m.fetchJobsCmd())
		}
		cmds = append(cmds, m.tickCmd())

		if m.inTailView {
			return m, tea.Batch(cmds...)
		}
	}

	if m.confirmingCancel {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "y", "Y":
				m.confirmingCancel = false
				if m.cancelCandidate != nil {
					cmds = append(cmds, m.cancelJobCmd(m.cancelCandidate.JobID))
					m.cancelCandidate = nil
				}
				return m, tea.Batch(cmds...)
			case "n", "N", "esc", "q":
				m.confirmingCancel = false
				m.cancelCandidate = nil
				return m, nil
			}
		}
		return m, nil
	}

	if m.inValueOverlay && !handledTick {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			width := msg.Width
			height := msg.Height
			if width <= 0 {
				if m.width > 0 {
					width = m.width
				} else {
					width, _ = detectTerminalSize()
				}
			}
			if height <= 0 {
				if m.height > 0 {
					height = m.height
				} else {
					_, height = detectTerminalSize()
				}
			}
			m.applyWindowSize(width, height)
			m.configureValueViewport()
			return m, nil
		case tea.KeyMsg:
			if key.Matches(msg, keys.CopyValue) {
				if strings.TrimSpace(m.valueValue) != "" {
					cmds = append(cmds, osc52CopyCmd(m.valueValue))
				}
				return m, tea.Batch(cmds...)
			}
			if key.Matches(msg, keys.ToggleHelp) {
				m.help.ShowAll = !m.help.ShowAll
				m.applyWindowSize(m.width, m.height)
				m.configureValueViewport()
				return m, nil
			}
			switch msg.String() {
			case "esc", "q", "v":
				m.inValueOverlay = false
				m.applyWindowSize(m.width, m.height)
				return m, nil
			}
		}
		m.valueView, cmd = m.valueView.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	if m.inDetailsOverlay && !handledTick {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			// Keep overlay responsive to resizes.
			width := msg.Width
			height := msg.Height
			if width <= 0 {
				if m.width > 0 {
					width = m.width
				} else {
					width, _ = detectTerminalSize()
				}
			}
			if height <= 0 {
				if m.height > 0 {
					height = m.height
				} else {
					_, height = detectTerminalSize()
				}
			}
			m.applyWindowSize(width, height)
			return m, nil
		case tea.KeyMsg:
			if key.Matches(msg, keys.ToggleHelp) {
				m.help.ShowAll = !m.help.ShowAll
				m.applyWindowSize(m.width, m.height)
				return m, nil
			}
			if key.Matches(msg, keys.CopyValue) {
				if cmd := m.copySelectedDetailCmd(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
			if key.Matches(msg, keys.ViewValue) {
				if cmd := m.openValueOverlayCmd(); cmd != nil {
					cmds = append(cmds, cmd)
					return m, tea.Batch(cmds...)
				}
			}
			// In overlay mode, treat Esc/q/i as "close overlay" instead of quitting.
			switch msg.String() {
			case "esc", "q", "i":
				m.inDetailsOverlay = false
				// Re-apply layout so widths/heights go back to normal.
				m.applyWindowSize(m.width, m.height)
				return m, nil
			}
		}

		// Allow scrolling/copying within the details table.
		m.detailsTable, cmd = m.detailsTable.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	if m.inTailView && !handledTick {
		wasInSearchMode := m.tailModel.InSearchMode()

		switch msg := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(msg, tailKeys.ToggleHelp) && !wasInSearchMode {
				m.help.ShowAll = !m.help.ShowAll
				return m, nil
			}
			if key.Matches(msg, tailKeys.Quit) && !wasInSearchMode {
				m.inTailView = false
				// Restore the pre-tail mouse setting (tail view may have
				// auto-disabled it).
				if m.mouseEnabled != m.mouseEnabledBeforeTail {
					m.mouseEnabled = m.mouseEnabledBeforeTail
					if m.mouseEnabled {
						cmds = append(cmds, tea.EnableMouseCellMotion)
					} else {
						cmds = append(cmds, tea.DisableMouse)
					}
				}
				// Re-trigger a job refresh when coming back
				cmds = append(cmds, m.fetchJobsCmd())
				// Refresh details to clear "Resolving logs..." status
				if m.selectedID != "" {
					cmds = append(cmds, m.fetchDetailsCmd(m.selectedID))
				}
			}
			// Capture mouse toggle from tail view to keep state in sync
			if key.Matches(msg, tailKeys.ToggleMouse) && !wasInSearchMode {
				m.mouseEnabled = !m.mouseEnabled
				if m.mouseEnabled {
					cmds = append(cmds, tea.EnableMouseCellMotion)
				} else {
					cmds = append(cmds, tea.DisableMouse)
				}
				// TailModel also needs to handle this to update its UI, so we continue
			}
		}

		var newTail tea.Model
		newTail, cmd = m.tailModel.Update(msg)
		m.tailModel = newTail.(TailModel)
		cmds = append(cmds, cmd)

		// Sync mouse state from tail model (e.g. if it auto-disabled mouse)
		if m.mouseEnabled != m.tailModel.mouseEnabled {
			m.mouseEnabled = m.tailModel.mouseEnabled
		}

		// Double check if we should exit based on the msg that was processed
		if msg, ok := msg.(tea.KeyMsg); ok && key.Matches(msg, tailKeys.Quit) && !wasInSearchMode {
			m.inTailView = false
			// Restore the pre-tail mouse setting (tail view may have
			// auto-disabled it).
			if m.mouseEnabled != m.mouseEnabledBeforeTail {
				m.mouseEnabled = m.mouseEnabledBeforeTail
				if m.mouseEnabled {
					cmds = append(cmds, tea.EnableMouseCellMotion)
				} else {
					cmds = append(cmds, tea.DisableMouse)
				}
			}
			// Refresh details here too just in case
			if m.selectedID != "" {
				cmds = append(cmds, m.fetchDetailsCmd(m.selectedID))
			}
			return m, tea.Batch(cmds...)
		}

		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Some terminals briefly report zero dimensions (e.g. during font or window
		// changes). Instead of ignoring these events entirely – which can leave the
		// UI in an uninitialized state until the next real resize – fall back to the
		// last known or a reasonable default size.
		width := msg.Width
		height := msg.Height
		if width <= 0 {
			if m.width > 0 {
				width = m.width
			} else {
				width, _ = detectTerminalSize()
			}
		}
		if height <= 0 {
			if m.height > 0 {
				height = m.height
			} else {
				_, height = detectTerminalSize()
			}
		}

		m.applyWindowSize(width, height)
		if m.inValueOverlay {
			m.configureValueViewport()
		}

	case jobsMsg:
		m.jobs = msg
		m.lastRefresh = time.Now()
		m.loadingJobs = false
		m.updateTable()

		// Sync selection immediately
		sel := m.table.SelectedRow()
		if len(sel) > 0 {
			id := sel[0]
			// If selection changed or we haven't loaded details yet (e.g. startup).
			// When details are hidden (small window), avoid fetching details on every
			// selection change; fetch on-demand when opening the overlay.
			if id != m.selectedID || m.selectedID == "" {
				m.selectedID = id
				if !m.hideDetails {
					cmds = append(cmds, m.fetchDetailsCmd(id))
				}
			}
		}

	case detailsMsg:
		m.rawDetails = string(msg)
		m.updateDetailsTable(m.rawDetails)

	case tailPathsMsg:
		// Use the job ID associated with the request; selection may have
		// changed while paths were resolving.
		if msg.jobID != "" {
			m.selectedID = msg.jobID
			m.setTableCursorByJobID(msg.jobID)
		}
		if msg.err != nil {
			m.err = msg.err
		}
		m.mouseEnabledBeforeTail = m.mouseEnabled
		m.tailModel = NewTailModel(m.selectedID, msg.stdout, msg.stderr, m.width, m.height, msg.mode)
		m.tailModel.mouseEnabled = m.mouseEnabled // Sync state
		m.inTailView = true
		cmds = append(cmds, m.tailModel.Init())

	case errMsg:
		m.err = msg

	case refreshNowMsg:
		// Trigger a refresh without spawning another tick loop.
		cmds = append(cmds, m.fetchJobsCmd())

	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft {
			if m.hideDetails {
				m.table.Focus()
				m.detailsTable.Blur()
				m.inputMode = false
				m.filterInput.Blur()
				break
			}
			// Determine which pane was clicked
			// Table is on the left, Width is m.table.Width() + chrome
			tableEnd := m.table.Width() + 4 // roughly

			if msg.X < tableEnd {
				m.table.Focus()
				m.detailsTable.Blur()
				m.inputMode = false
				m.filterInput.Blur()
			} else {
				m.table.Blur()
				m.detailsTable.Focus()
				m.inputMode = false
				m.filterInput.Blur()
			}
		}

	case tea.KeyMsg:
		if m.inputMode {
			switch msg.String() {
			case "enter", "esc":
				m.inputMode = false
				m.table.Focus()
				m.filterInput.Blur()
			default:
				m.filterInput, cmd = m.filterInput.Update(msg)
				cmds = append(cmds, cmd)
				m.updateTable()
				return m, tea.Batch(cmds...)
			}
		} else {
			switch {
			case key.Matches(msg, keys.ToggleHelp):
				m.help.ShowAll = !m.help.ShowAll
				m.applyWindowSize(m.width, m.height)
				return m, nil
			case key.Matches(msg, keys.Quit):
				return m, tea.Quit
			case key.Matches(msg, keys.Filter):
				m.inputMode = true
				m.filterInput.Focus()
				m.table.Blur()
				return m, nil
			case key.Matches(msg, keys.Pause):
				m.paused = !m.paused
			case key.Matches(msg, keys.Refresh):
				cmds = append(cmds, m.fetchJobsCmd())
			case key.Matches(msg, keys.History):
				m.loadingJobs = true
				if m.appMode == modeLive {
					m.appMode = modeHistory
				} else {
					m.appMode = modeLive
				}
				cmds = append(cmds, m.fetchJobsCmd())
				cmds = append(cmds, func() tea.Msg {
					return tea.WindowSizeMsg{Width: m.width, Height: m.height}
				})
			case key.Matches(msg, keys.StatusFilter):
				m.sFilter = (m.sFilter + 1) % 3
				m.updateTable()
			case key.Matches(msg, keys.InspectJob):
				job := m.getSelectedJob()
				if job != nil {
					// In small windows the details panel is hidden; use a full-screen overlay.
					if m.hideDetails {
						m.inDetailsOverlay = true
						m.detailsTable.Focus()
						m.table.Blur()
						cmds = append(cmds, m.fetchDetailsCmd(job.JobID))
						// Force layout recalculation so the overlay columns fit the window.
						m.applyWindowSize(m.width, m.height)
						return m, tea.Batch(cmds...)
					}
					cmds = append(cmds, m.fetchDetailsCmd(job.JobID))
				}
			case key.Matches(msg, keys.CancelJob):
				job := m.getSelectedJob()
				if job != nil {
					m.cancelCandidate = job
					m.confirmingCancel = true
				}
			case key.Matches(msg, keys.TailLogs):
				job := m.getSelectedJob()
				if job != nil {
					// Provide feedback in details table immediately
					m.detailsTable.SetRows([]table.Row{{"Status", "Resolving logs..."}})
					// Trigger the command
					cmds = append(cmds, m.resolveTailPathsCmd(job.JobID, TailModeBoth))
				}
			case key.Matches(msg, keys.TailStdout):
				job := m.getSelectedJob()
				if job != nil {
					m.detailsTable.SetRows([]table.Row{{"Status", "Resolving stdout..."}})
					cmds = append(cmds, m.resolveTailPathsCmd(job.JobID, TailModeStdout))
				}
			case key.Matches(msg, keys.TailStderr):
				job := m.getSelectedJob()
				if job != nil {
					m.detailsTable.SetRows([]table.Row{{"Status", "Resolving stderr..."}})
					cmds = append(cmds, m.resolveTailPathsCmd(job.JobID, TailModeStderr))
				}
			case key.Matches(msg, keys.SwitchFocus):
				if m.hideDetails {
					// Details panel isn't visible; keep focus on the jobs table.
					m.table.Focus()
					m.detailsTable.Blur()
					break
				}
				if m.table.Focused() {
					m.table.Blur()
					m.detailsTable.Focus()
				} else {
					m.detailsTable.Blur()
					m.table.Focus()
				}
			case key.Matches(msg, keys.ToggleMouse):
				m.mouseEnabled = !m.mouseEnabled
				if m.mouseEnabled {
					cmds = append(cmds, tea.EnableMouseCellMotion)
				} else {
					cmds = append(cmds, tea.DisableMouse)
				}
			case key.Matches(msg, keys.CopyValue):
				if m.hideDetails {
					m.copyFeedback = "Open details ('i') to copy values"
					m.copyFeedbackExpiry = time.Now().Add(2 * time.Second)
					break
				}
				if cmd := m.copySelectedDetailCmd(); cmd != nil {
					cmds = append(cmds, cmd)
				}
			case key.Matches(msg, keys.ViewValue):
				if cmd := m.openValueOverlayCmd(); cmd != nil {
					cmds = append(cmds, cmd)
					return m, tea.Batch(cmds...)
				}
			}
		}
	}

	if !m.inputMode {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)

		sel := m.table.SelectedRow()
		if len(sel) > 0 {
			id := sel[0]
			if id != m.selectedID {
				m.selectedID = id
				if !m.hideDetails {
					cmds = append(cmds, m.fetchDetailsCmd(id))
				}
			}
		}
	}

	m.detailsTable, cmd = m.detailsTable.Update(msg)
	cmds = append(cmds, cmd)

	m.applyPanelHeights()

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.inTailView {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.tailModel.View(),
			m.help.View(tailKeys),
		)
	}

	if m.inValueOverlay {
		return m.viewValueOverlay()
	}

	if m.inDetailsOverlay {
		return m.viewDetailsOverlay()
	}

	if m.confirmingCancel && m.cancelCandidate != nil {
		msg := fmt.Sprintf("Are you sure you want to cancel job?\n\n%s (%s)\n\n[y/N]", m.cancelCandidate.JobID, m.cancelCandidate.Name)
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			dialogStyle.Render(msg),
		)
	}

	header := m.renderHeaderArea()
	tablePanel := m.renderTablePanel()
	mainView := tablePanel
	if !m.hideDetails {
		detailsPanel := m.renderDetailsPanel()
		mainView = m.renderMainContent(tablePanel, detailsPanel)
	}

	var helpKeys help.KeyMap = keys
	if m.inTailView {
		helpKeys = tailKeys
	}
	helpSection := m.help.View(helpKeys)

	sections := []string{header, mainView, helpSection}
	if hint := m.filterHint(); hint != "" {
		sections = append(sections, hint)
	}
	if hint := m.detailsHiddenHint(); hint != "" {
		sections = append(sections, hint)
	}

	fullView := lipgloss.JoinVertical(lipgloss.Left, sections...)
	fullView = clampViewHeight(fullView, m.height)
	fullView = clampViewWidth(fullView, m.width)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, fullView)
}

func (m Model) renderHeaderArea() string {
	modeStr := "Live"
	if m.appMode == modeHistory {
		modeStr = "History"
	}

	filterInput := filterBoxStyle.Render(m.filterInput.View())
	required := []string{
		filterInput,
		metaMutedPillStyle.Render("Status " + m.sFilter.String()),
		metaPillStyle.Render("Mode " + modeStr),
	}

	if m.paused {
		required = append(required, metaMutedPillStyle.Copy().Background(accentOrange).Render("Paused"))
	}
	if m.err != nil {
		errText := fmt.Sprintf("Error %s", shortenText(m.err.Error(), 32))
		required = append(required, metaAlertPillStyle.Render(errText))
	}

	optional := []string{}

	// Job stats: chips in wide terminals, compact pill in medium ones.
	if m.width >= 120 {
		optional = append(optional, joinWithGap(m.jobStatChips(), 0))
	} else if m.width >= 90 {
		if compact := m.jobStatsCompactPill(); compact != "" {
			optional = append(optional, compact)
		}
	}

	if !m.lastRefresh.IsZero() {
		optional = append(optional, metaMutedPillStyle.Render("Updated "+m.lastRefresh.Format("15:04:05")))
	}
	mouseState := "Mouse Off"
	if m.mouseEnabled {
		mouseState = "Mouse On"
	}
	optional = append(optional, metaMutedPillStyle.Render(mouseState))

	// Keep a one-line header by dropping optional items until it fits.
	parts := append([]string{}, required...)
	parts = append(parts, optional...)
	for len(parts) > 0 && lipgloss.Width(joinWithGap(parts, 1)) > m.width {
		// Drop lowest priority item (last).
		parts = parts[:len(parts)-1]
	}

	row := joinWithGap(parts, 1)
	return lipgloss.NewStyle().MaxWidth(m.width).Render(row)
}

func (m Model) filterHint() string {
	if m.inputMode || m.filterInput.Value() != "" {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(filterHintStyle.Render("Press '/' to focus the filter"))
}

func (m Model) detailsHiddenHint() string {
	if !m.hideDetails || m.inDetailsOverlay || m.inTailView {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(
		filterHintStyle.Render("Details hidden in small window - press 'i' or Enter to open"),
	)
}

func (m Model) tablePanelTitle() string {
	title := panelTitleStyle.Render(fmt.Sprintf("Jobs (%d)", len(m.filtered)))
	if m.table.Focused() && !m.inputMode {
		title = lipgloss.JoinHorizontal(lipgloss.Left, title, focusTagStyle.Render("Jobs Focused"))
	}
	return title
}

func (m Model) jobStatsCompactPill() string {
	stats := m.collectJobStats()
	parts := []string{}
	if stats.Running > 0 {
		parts = append(parts, fmt.Sprintf("R%d", stats.Running))
	}
	if stats.Pending > 0 {
		parts = append(parts, fmt.Sprintf("P%d", stats.Pending))
	}
	if stats.Failed > 0 {
		parts = append(parts, fmt.Sprintf("F%d", stats.Failed))
	}
	if stats.Completed > 0 {
		parts = append(parts, fmt.Sprintf("C%d", stats.Completed))
	}
	if stats.Other > 0 {
		parts = append(parts, fmt.Sprintf("O%d", stats.Other))
	}
	if len(parts) == 0 {
		return ""
	}
	return metaMutedPillStyle.Render(strings.Join(parts, " "))
}

func (m Model) renderTablePanel() string {
	tableStyle := m.tableBoxStyle()
	if m.tableBlockWidth > 0 {
		tableStyle = tableStyle.Width(m.tableBlockWidth)
	}

	tableFocused := m.table.Focused() && !m.inputMode
	if tableFocused {
		tableStyle = tableStyle.BorderForeground(highlight).Background(panelBg)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.tablePanelTitle(),
		tableStyle.Render(m.table.View()),
	)
}

func (m Model) detailsPanelTitle() string {
	title := panelTitleStyle.Render("Details")
	if m.detailsTable.Focused() {
		title = lipgloss.JoinHorizontal(lipgloss.Left, title, focusTagStyle.Render("Details Focused"))
	} else {
		hint := placeholderStyle.Copy().MarginLeft(1).Render("Press TAB to scroll")
		title = lipgloss.JoinHorizontal(lipgloss.Left, title, hint)
	}
	return title
}

func (m Model) renderDetailsPanel() string {
	title := m.detailsPanelTitle()

	// FIX 3: Change .MaxWidth() to .Width()
	// This forces Lipgloss to draw the box at exactly this size,
	// preventing it from shrinking or behaving unpredictably.
	panelStyle := m.detailsBoxStyle().Width(m.detailsBlockWidth)

	if m.detailsTable.Focused() {
		panelStyle = panelStyle.BorderForeground(highlight).Background(panelBg)
	}

	detailsContent := m.detailsTable.View()
	if strings.TrimSpace(detailsContent) == "" {
		detailsContent = placeholderStyle.Render("Details will appear here once a job is selected.")
	}

	if inspector, _ := m.buildDetailInspector(); inspector != "" {
		detailsContent = lipgloss.JoinVertical(lipgloss.Left, detailsContent, inspector)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		panelStyle.Render(detailsContent),
	)
}

func (m Model) viewDetailsOverlay() string {
	header := metaPillStyle.Copy().
		Foreground(textStrong).
		BorderForeground(panelBorder).
		Render(fmt.Sprintf("Details %s", m.selectedID))
	hint := metaMutedPillStyle.Render(detailsOverlayHintText(m.width))

	var top string
	if m.width < 90 {
		top = lipgloss.JoinVertical(lipgloss.Left, header, hint)
	} else {
		top = joinWithGap([]string{header, hint}, 1)
	}
	top = lipgloss.NewStyle().MaxWidth(m.width).Render(top)

	// Allocate remaining height to the table.
	reserved := lipgloss.Height(top) + lipgloss.Height(m.help.View(keys))
	bodyH := m.height - reserved
	if bodyH < 5 {
		bodyH = 5
	}

	// Full-width details table.
	w := m.width - panelChromeWidth
	if w < 10 {
		w = 10
	}
	m.detailsTable.SetWidth(w)
	keyW := (w * 25) / 100
	if keyW < 8 {
		keyW = 8
	}
	valW := w - keyW - 1
	if valW < 1 {
		valW = 1
	}
	m.detailsTable.SetColumns([]table.Column{
		{Title: "Key", Width: keyW},
		{Title: "Value", Width: valW},
	})
	m.detailsTable.SetHeight(bodyH - 3)

	panel := m.detailsBoxStyle().Width(m.width - 2).Render(m.detailsTable.View())

	view := lipgloss.JoinVertical(lipgloss.Left, top, panel, m.help.View(keys))
	view = clampViewHeight(view, m.height)
	view = clampViewWidth(view, m.width)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, view)
}

func (m *Model) openValueOverlayCmd() tea.Cmd {
	// Ensure we have a selected detail row.
	keyText, valueText, ok := m.selectedDetailEntry()
	if !ok {
		m.copyFeedback = "Select a detail row to view"
		m.copyFeedbackExpiry = time.Now().Add(2 * time.Second)
		return nil
	}

	m.inValueOverlay = true
	m.valueKey = keyText
	m.valueValue = valueText
	m.configureValueViewport()
	return nil
}

func (m *Model) configureValueViewport() {
	// Leave room for a small header and the normal help row.
	headerH := 2
	helpH := lipgloss.Height(m.help.View(keys))
	h := m.height - headerH - helpH - 2
	if h < 5 {
		h = 5
	}
	w := m.width - panelChromeWidth
	if w < 10 {
		w = 10
	}

	m.valueView = viewport.New(w, h)
	content := m.valueValue
	if strings.TrimSpace(content) == "" {
		content = "(empty)"
	}
	m.valueView.SetContent(wordwrap.String(content, w))
	m.valueView.GotoTop()
}

func (m Model) viewValueOverlay() string {
	title := metaPillStyle.Copy().
		Foreground(textStrong).
		BorderForeground(panelBorder).
		Render(fmt.Sprintf("%s", m.valueKey))
	hint := metaMutedPillStyle.Render(valueOverlayHintText(m.width))
	var top string
	if m.width < 70 {
		top = lipgloss.JoinVertical(lipgloss.Left, title, hint)
	} else {
		top = joinWithGap([]string{title, hint}, 1)
	}
	top = lipgloss.NewStyle().MaxWidth(m.width).Render(top)

	panel := m.detailsBoxStyle().Width(m.width - 2).Render(m.valueView.View())

	view := lipgloss.JoinVertical(lipgloss.Left, top, panel, m.help.View(keys))
	view = clampViewHeight(view, m.height)
	view = clampViewWidth(view, m.width)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, view)
}

func (m Model) renderMainContent(tablePanel, detailsPanel string) string {
	if m.stackPanels {
		if m.stackGapHeight > 0 {
			gap := lipgloss.NewStyle().Height(m.stackGapHeight).Render(" ")
			return lipgloss.JoinVertical(lipgloss.Left, tablePanel, gap, detailsPanel)
		}
		return lipgloss.JoinVertical(lipgloss.Left, tablePanel, detailsPanel)
	}

	gap := lipgloss.NewStyle().Width(panelGap).Render(" ")
	return lipgloss.JoinHorizontal(lipgloss.Top, tablePanel, gap, detailsPanel)
}

func (m Model) selectedDetailEntry() (string, string, bool) {
	row := m.detailsTable.SelectedRow()
	if len(row) < 2 {
		return "", "", false
	}
	key := strings.TrimSpace(row[0])
	value := strings.TrimSpace(row[1])
	if value == "" {
		return "", "", false
	}
	return key, value, true
}

func (m *Model) copySelectedDetailCmd() tea.Cmd {
	_, value, ok := m.selectedDetailEntry()
	if !ok {
		m.copyFeedback = "No value to copy"
		m.copyFeedbackExpiry = time.Now().Add(2 * time.Second)
		return nil
	}

	m.copyFeedback = "Value copied"
	m.copyFeedbackExpiry = time.Now().Add(2 * time.Second)
	return osc52CopyCmd(value)
}

func (m Model) buildDetailInspector() (string, int) {
	_, _, ok := m.selectedDetailEntry()
	if !ok {
		return "", 0
	}

	contentWidth := m.detailsContentWidth

	copyMessage := detailInspectorHintText(contentWidth)
	copyStyle := copyHintStyle
	if m.copyFeedback != "" {
		copyMessage = m.copyFeedback
		copyStyle = copyStatusStyle
	}
	if contentWidth > 0 {
		copyMessage = trimDetailValueToWidth(copyMessage, contentWidth)
	}

	metaRowStyle := lipgloss.NewStyle().Background(panelBgAccent)
	if contentWidth > 0 {
		metaRowStyle = metaRowStyle.Width(contentWidth)
	}
	metaRow := metaRowStyle.Render(copyStyle.Render(copyMessage))

	content := metaRow

	inspectorStyle := detailInspectorStyle.Copy()
	if contentWidth > 0 {
		inspectorStyle = inspectorStyle.Width(contentWidth)
	}

	view := inspectorStyle.Render(content)
	return view, lipgloss.Height(view)
}

func detailInspectorHintText(width int) string {
	switch {
	case width >= 42:
		return "Press v to view full value  •  Ctrl+Y to copy"
	case width >= 28:
		return "v view value  •  Ctrl+Y copy"
	case width >= 16:
		return "v view  •  ^Y copy"
	default:
		return "v/^Y"
	}
}

func detailsOverlayHintText(width int) string {
	switch {
	case width >= 56:
		return "Esc/q/i close  •  v view full value  •  Ctrl+Y copy"
	case width >= 34:
		return "Esc/q/i close  •  v view  •  ^Y copy"
	default:
		return "Esc/q/i  •  v/^Y"
	}
}

func valueOverlayHintText(width int) string {
	switch {
	case width >= 42:
		return "Esc/q/v close  •  Ctrl+Y copy"
	case width >= 24:
		return "Esc/q/v  •  ^Y copy"
	default:
		return "Esc/q/v  •  ^Y"
	}
}

func sanitizeDetailValue(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\t", " ")
	return strings.TrimSpace(value)
}

func trimDetailValueToWidth(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return runewidth.Truncate(value, 1, "")
	}
	return runewidth.Truncate(value, width, "…")
}

func clampViewWidth(view string, width int) string {
	if width <= 0 {
		return view
	}
	lines := strings.Split(strings.ReplaceAll(view, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			lines[i] = truncate.String(line, uint(width))
		}
	}
	return strings.Join(lines, "\n")
}

func clampViewHeight(view string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(view, "\r\n", "\n"), "\n")
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:height], "\n")
}

func (m *Model) applyPanelHeights() {
	tableHeight := m.tablePanelHeight
	detailsHeight := m.detailsPanelHeight

	if tableHeight < 0 {
		tableHeight = 0
	}
	if detailsHeight < 0 {
		detailsHeight = 0
	}

	tableTitleHeight := lipgloss.Height(m.tablePanelTitle())
	_, tableFrameHeight := m.tableBoxStyle().GetFrameSize()
	tableContentHeight := tableHeight - tableTitleHeight - tableFrameHeight
	if tableContentHeight < 0 {
		tableContentHeight = 0
	}
	m.table.SetHeight(tableContentHeight)

	detailsTitleHeight := lipgloss.Height(m.detailsPanelTitle())
	_, detailsFrameHeight := m.detailsBoxStyle().GetFrameSize()
	_, inspectorHeight := m.buildDetailInspector()
	detailsContentHeight := detailsHeight - detailsTitleHeight - detailsFrameHeight
	if inspectorHeight > 0 {
		detailsContentHeight -= inspectorHeight
	}
	if detailsContentHeight < 0 {
		detailsContentHeight = 0
	}
	m.detailsTable.SetHeight(detailsContentHeight)
}

func (m *Model) applyWindowSize(width, height int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	m.width = width
	m.height = height

	// --- 1. HEADER & INPUT SIZING ---
	m.help.Width = width - 2

	// Responsive filter width: keep it compact on small windows, roomier on wide ones.
	switch {
	case width >= 110:
		m.filterInput.Width = 20
	case width >= 80:
		m.filterInput.Width = 12
	default:
		m.filterInput.Width = 10
	}

	// --- 2. VERTICAL HEIGHT CALCULATION ---
	headerHeight := lipgloss.Height(m.renderHeaderArea())
	helpHeight := lipgloss.Height(m.help.View(keys))
	hintHeight := 0
	if hint := m.filterHint(); hint != "" {
		hintHeight = lipgloss.Height(hint)
	}

	reserved := headerHeight + helpHeight + hintHeight
	availableHeight := height - reserved
	if availableHeight < 0 {
		availableHeight = 0
	}

	// --- 3. PANEL WIDTH CALCULATION ---

	safeWidth := width

	// FIX: Use -6 buffer.
	// This ensures the rightmost border is definitely within the screen bounds.
	usable := safeWidth - panelGap - 6

	if usable < 1 {
		usable = 1
	}

	minCombinedWidth := minTablePanelWidth + minDetailsPanelWidth
	stackPanels := safeWidth < minCombinedWidth+panelGap
	m.stackPanels = stackPanels

	var tableBlockWidth, detailsBlockWidth int

	// Hide details entirely in small windows. Users can open a full-screen overlay via 'i'/Enter.
	// This prioritizes the jobs table, which is the primary workflow.
	hideDetails := stackPanels || availableHeight < 14
	m.hideDetails = hideDetails

	if hideDetails {
		tableBlockWidth = safeWidth - 2
		if tableBlockWidth < 1 {
			tableBlockWidth = 1
		}
		detailsBlockWidth = safeWidth - 2
		m.tablePanelHeight = availableHeight
		m.detailsPanelHeight = 0
		m.stackGapHeight = 0
	} else if stackPanels {
		tableBlockWidth = safeWidth - 2
		detailsBlockWidth = safeWidth - 2
		m.tablePanelHeight = availableHeight / 2
		m.detailsPanelHeight = availableHeight - m.tablePanelHeight
		m.stackGapHeight = 1
	} else {
		// Set Table to 60%
		tableBlockWidth = (usable * 60) / 100

		if tableBlockWidth < minTablePanelWidth {
			tableBlockWidth = minTablePanelWidth
		}

		detailsBlockWidth = usable - tableBlockWidth

		// Ensure details panel has minimum space
		if detailsBlockWidth < minDetailsPanelWidth {
			detailsBlockWidth = minDetailsPanelWidth
			tableBlockWidth = usable - detailsBlockWidth
		}

		// Cap details width so it doesn't dominate wide terminals.
		if detailsBlockWidth > maxDetailsPanelWidth {
			detailsBlockWidth = maxDetailsPanelWidth
			tableBlockWidth = usable - detailsBlockWidth
			if tableBlockWidth < minTablePanelWidth {
				tableBlockWidth = minTablePanelWidth
				detailsBlockWidth = usable - tableBlockWidth
			}
		}

		m.tablePanelHeight = availableHeight
		m.detailsPanelHeight = availableHeight
		m.stackGapHeight = 0
	}

	m.tableBlockWidth = tableBlockWidth
	m.detailsBlockWidth = detailsBlockWidth

	// --- 4. INTERNAL CONTENT SIZING ---
	// Compute content widths based on actual frame sizes so borders never
	// overflow the terminal.
	tableFrameX, _ := m.tableBoxStyle().GetFrameSize()
	detailsFrameX, _ := m.detailsBoxStyle().GetFrameSize()

	tableContentWidth := tableBlockWidth - tableFrameX
	if tableContentWidth < 1 {
		tableContentWidth = 1
	}

	detailsContentWidth := detailsBlockWidth - detailsFrameX
	if detailsContentWidth < 1 {
		detailsContentWidth = 1
	}
	m.detailsContentWidth = detailsContentWidth

	// When changing the number of columns we must ensure that the current rows
	// do not have more cells than there are columns. The bubbles table
	// implementation indexes columns by the row cell index during rendering,
	// so shrinking the column set before updating the rows can cause an
	// out-of-range panic during a resize. Clear rows first so that any
	// subsequent SetColumns call operates on an empty table, then rebuild rows
	// via updateTable below.
	m.table.SetRows([]table.Row{})

	// Hide columns if super narrow
	m.table.SetColumns(m.responsiveTableColumns(tableContentWidth))
	m.table.SetWidth(tableContentWidth)
	m.updateTable()

	m.detailsTable.SetWidth(detailsContentWidth)

	// Adjust Detail Column Ratios
	keyWidth := (detailsContentWidth * 30) / 100
	if keyWidth < 8 {
		keyWidth = 8
	}
	valWidth := detailsContentWidth - keyWidth - 1
	if valWidth < 1 {
		valWidth = 1
	}

	m.detailsTable.SetColumns([]table.Column{
		{Title: "Key", Width: keyWidth},
		{Title: "Value", Width: valWidth},
	})

	if m.rawDetails != "" {
		m.updateDetailsTable(m.rawDetails)
	}

	m.applyPanelHeights()
}

func (m Model) panelPadding() (padY, padX int) {
	// Tighten chrome when space is limited.
	if m.width < 90 || m.height < 26 || m.stackPanels {
		return 0, 1
	}
	return 1, 2
}

func (m Model) tableBoxStyle() lipgloss.Style {
	padY, padX := m.panelPadding()
	return listStyle.Copy().Padding(padY, padX)
}

func (m Model) detailsBoxStyle() lipgloss.Style {
	padY, padX := m.panelPadding()
	return detailsStyle.Copy().Padding(padY, padX)
}

type jobStats struct {
	Running   int
	Pending   int
	Completed int
	Failed    int
	Other     int
}

func (m Model) collectJobStats() jobStats {
	stats := jobStats{}
	for _, j := range m.filtered {
		state := j.State()
		switch state {
		case "R", "CG":
			stats.Running++
		case "PD", "CF", "PR", "RQ", "RS", "S", "ST", "RH", "RF":
			stats.Pending++
		case "CD":
			stats.Completed++
		case "F", "TO", "NF", "OOM", "CA":
			stats.Failed++
		default:
			stats.Other++
		}
	}
	return stats
}

func (m Model) jobStatChips() []string {
	stats := m.collectJobStats()
	metrics := []struct {
		short string
		label string
		icon  string
		value int
		color lipgloss.TerminalColor
	}{
		{"R", "Running", "▶", stats.Running, accentGreen},
		{"P", "Pending", "…", stats.Pending, accentOrange},
		{"C", "Completed", "✓", stats.Completed, accentBlue},
		{"F", "Failed", "!", stats.Failed, accentPink},
		{"O", "Other", "?", stats.Other, accentCyan},
	}

	var chips []string
	for _, metric := range metrics {
		if metric.label == "Other" && metric.value == 0 {
			continue
		}

		value := summaryValueStyle.Copy().Foreground(metric.color).Render(fmt.Sprintf("%s %d", metric.icon, metric.value))
		content := lipgloss.JoinHorizontal(
			lipgloss.Left,
			summaryLabelStyle.Render(metric.short),
			lipgloss.NewStyle().MarginLeft(1).Render(value),
		)
		chips = append(chips, summaryChipStyle.Copy().BorderForeground(metric.color).Render(content))
	}

	if len(chips) == 0 {
		chips = append(chips, summaryChipStyle.Render(placeholderStyle.Render("No jobs to display")))
	}

	return chips
}

func (m Model) responsiveTableColumns(contentWidth int) []table.Column {
	// Build a column set that degrades gracefully in small windows.
	// We keep Name flexible to absorb extra space.
	usable := contentWidth - 2 // small safety margin
	if usable < 1 {
		usable = 1
	}

	idW := 8
	statusW := 6
	nameMin := 12

	type optCol struct {
		title string
		width int
	}
	optionals := []optCol{
		{"Time", 10},
		{"Nodes", 6},
		{"Partition", 10},
		{"Nodelist", 15},
	}

	sumOther := idW + statusW
	var chosen []optCol
	for _, c := range optionals {
		if usable-(sumOther+c.width) >= nameMin {
			chosen = append(chosen, c)
			sumOther += c.width
		}
	}

	nameW := usable - sumOther
	if nameW < nameMin {
		nameW = nameMin
	}
	cols := []table.Column{
		{Title: "Job ID", Width: idW},
		{Title: "Name", Width: nameW},
		{Title: "Status", Width: statusW},
	}
	for _, c := range chosen {
		cols = append(cols, table.Column{Title: c.title, Width: c.width})
	}
	return cols
}

func joinWithGap(parts []string, gap int) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	if gap <= 0 {
		return lipgloss.JoinHorizontal(lipgloss.Left, filtered...)
	}
	spacer := lipgloss.NewStyle().Width(gap).Render(" ")
	row := filtered[0]
	for _, part := range filtered[1:] {
		row = lipgloss.JoinHorizontal(lipgloss.Left, row, spacer, part)
	}
	return row
}

func renderStateBadge(state, label string) string {
	code := strings.ToUpper(strings.TrimSpace(state))
	if code == "" && label != "" {
		code = StateCode(label)
	}

	caption := strings.TrimSpace(label)
	if caption == "" {
		caption = strings.ToUpper(code)
	} else {
		caption = strings.ToUpper(caption)
		if code != "" {
			caption = fmt.Sprintf("%s (%s)", caption, code)
		}
	}
	if caption == "" {
		caption = "UNKNOWN"
	}

	return statusBadgeStyle.Copy().
		Background(statusColor(code)).
		Render(caption)
}

func shortenText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// --- Helpers ---

func (m *Model) updateDetailsTable(text string) {
	var rows []table.Row
	if m.appMode == modeHistory {
		rows = parseHistoryDetailsToRows(text)
	} else {
		rows = parseDetailsToRows(text)
	}
	m.detailsTable.SetRows(rows)
}

func parseDetailsToRows(text string) []table.Row {
	var rows []table.Row

	// Handle potential error messages
	if strings.HasPrefix(text, "Error") {
		return []table.Row{{"Error", text}}
	}

	// Heuristic parsing for Key=Value pairs
	// 1. Replace newlines with spaces to handle multi-line output effectively?
	//    But 'scontrol show job' output is structured with newlines.
	//    Let's process line by line.

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by whitespace, but respect key=value
		// This is naive.
		// Better: Regex `(\w+(?:[:_]\w+)*)=`
		// But we can just iterate fields.

		fields := strings.Fields(line)
		for _, field := range fields {
			parts := strings.SplitN(field, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				val := parts[1]
				if val == "" {
					val = "(empty)"
				}
				rows = append(rows, table.Row{key, val})
			} else {
				// Maybe part of previous value?
				// For now, ignore or append to last row?
				// Simple approach: if not k=v, ignore.
			}
		}
	}

	// Optimization: Filter out common boring keys if needed?
	// For now show all.

	return rows
}

func parseHistoryDetailsToRows(text string) []table.Row {
	text = strings.TrimSpace(text)
	if text == "" {
		return []table.Row{{"Info", "No history details available"}}
	}

	if strings.HasPrefix(text, "Error") {
		return []table.Row{{"Error", text}}
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return []table.Row{{"Info", "No history details available"}}
	}

	var fields []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			// Unexpected format; fall back to generic parser.
			return parseDetailsToRows(text)
		}

		jobID := strings.TrimSpace(parts[0])
		if jobID == "" {
			continue
		}

		// Prefer the top-level job row (no step suffix like ".batch").
		if !strings.Contains(jobID, ".") {
			fields = parts
			break
		}

		// Fallback: remember the first non-empty row if we never see a top-level one.
		if len(fields) == 0 {
			fields = parts
		}
	}

	if len(fields) == 0 {
		return []table.Row{{"Info", "No history details found"}}
	}

	labels := []string{
		"JobID",
		"JobName",
		"User",
		"State",
		"Partition",
		"Elapsed",
		"AllocNodes",
		"NodeList",
		"Start",
		"End",
		"ExitCode",
	}

	var rows []table.Row
	for i, label := range labels {
		if i >= len(fields) {
			break
		}
		val := strings.TrimSpace(fields[i])
		if val == "" {
			continue
		}
		rows = append(rows, table.Row{label, val})

		// Add a normalized short state code to make it easy
		// to correlate with the dashboard’s badges.
		if label == "State" {
			if code := StateCode(val); code != "" && code != val {
				rows = append(rows, table.Row{"StateCode", code})
			}
		}
	}

	if len(rows) == 0 {
		return []table.Row{{"Info", "No history details found"}}
	}
	return rows
}

func (m *Model) getSelectedJob() *Job {
	// Always query the table for the currently selected row to ensure we have the latest selection
	// and to avoid issues where m.selectedID might be stale or uninitialized.
	sel := m.table.SelectedRow()
	if len(sel) == 0 {
		return nil
	}

	id := sel[0]
	for i := range m.jobs {
		if m.jobs[i].JobID == id {
			return &m.jobs[i]
		}
	}
	return nil
}

func (m *Model) setTableCursorByJobID(jobID string) {
	if jobID == "" {
		return
	}
	rows := m.table.Rows()
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		if strings.TrimSpace(row[0]) == jobID {
			m.table.SetCursor(i)
			return
		}
	}
}

func (m *Model) updateTable() {
	if m.loadingJobs {
		// Keep existing rows while a new job list is being fetched
		return
	}

	m.filtered = []Job{}
	query := strings.ToLower(m.filterInput.Value())

	for _, j := range m.jobs {
		if m.appMode == modeHistory && !j.IsHistorical() {
			continue
		}
		if m.sFilter == filterRunning && !j.IsRunning() {
			continue
		}
		if m.sFilter == filterPending && !j.IsPending() {
			continue
		}

		if query != "" {
			if !strings.Contains(strings.ToLower(j.Name), query) &&
				!strings.Contains(j.JobID, query) {
				continue
			}
		}
		m.filtered = append(m.filtered, j)
	}

	rows := []table.Row{}

	// Helper to truncate strings
	truncate := func(s string, max int) string {
		if len(s) > max {
			if max > 3 {
				return s[:max-3] + "..."
			}
			return s[:max]
		}
		return s
	}

	for _, j := range m.filtered {
		status := j.State()
		// Note: ANSI colors removed as they interfere with table column width calculation
		// causing truncation (e.g. "P...") and layout shifting.

		// Create row based on current columns
		// We must match the number of columns currently set in the table
		currentCols := m.table.Columns()

		// Standard row data
		fullRow := []string{
			j.JobID,
			j.Name,
			truncate(status, 12),
			truncate(j.Time, 12),
			truncate(j.Nodes, 8),
			truncate(j.Partition, 12),
			truncate(j.NodeList, 20),
		}

		// Slice row data to match column count
		// This prevents "index out of range" if we are in compact mode
		if len(currentCols) < len(fullRow) {
			rows = append(rows, fullRow[:len(currentCols)])
		} else {
			rows = append(rows, fullRow)
		}
	}
	m.table.SetRows(rows)
}

// --- Commands ---

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func initialWindowSizeCmd() tea.Cmd {
	return func() tea.Msg {
		width, height := detectTerminalSize()
		return tea.WindowSizeMsg{Width: width, Height: height}
	}
}

func detectTerminalSize() (int, int) {
	width, height, err := term.GetSize(os.Stdout.Fd())
	if err != nil || width <= 0 || height <= 0 {
		return 80, 24
	}
	return width, height
}

func historyDaysFromEnv() int {
	raw := strings.TrimSpace(os.Getenv(envHistoryDays))
	if raw == "" {
		return defaultHistoryDays
	}

	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		return defaultHistoryDays
	}
	return days
}

func (m Model) fetchJobsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.appMode == modeHistory {
			jobs, err := FetchJobsHistory(m.historyDays)
			if err != nil {
				return errMsg(err)
			}
			return jobsMsg(jobs)
		}
		jobs, err := FetchJobsSqueue()
		if err != nil {
			return errMsg(err)
		}
		return jobsMsg(jobs)
	}
}

func (m Model) fetchDetailsCmd(id string) tea.Cmd {
	return func() tea.Msg {
		det, err := GetJobDetails(id, m.appMode == modeHistory)
		if err != nil {
			return detailsMsg(fmt.Sprintf("Error fetching details: %v", err))
		}
		return detailsMsg(det)
	}
}

func (m Model) cancelJobCmd(id string) tea.Cmd {
	return func() tea.Msg {
		err := CancelJob(id)
		if err != nil {
			return errMsg(err)
		}
		return refreshNowMsg{}
	}
}

func (m Model) resolveTailPathsCmd(id string, mode TailMode) tea.Cmd {
	return func() tea.Msg {
		out, errPath, errExec := ResolveLogPaths(id)

		// If resolution failed entirely, return empty paths
		// The tail view will show "No path provided" for empty paths
		if errExec != nil {
			// Return empty paths - the tail view handles this gracefully. We
			// also propagate the error so the header can show it.
			return tailPathsMsg{jobID: id, stdout: "", stderr: "", mode: mode, err: errExec}
		}

		return tailPathsMsg{jobID: id, stdout: out, stderr: errPath, mode: mode}
	}
}

func main() {
	p := tea.NewProgram(NewModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
