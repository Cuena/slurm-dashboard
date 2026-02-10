package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// MaxLogLines caps the number of log lines kept in memory per pane.
// Keeping this reasonably small avoids unbounded memory growth and slow re-renders
// when viewing very large logs. Increase if you need more history.
const MaxLogLines = 5000

type TailMode int

const (
	TailModeBoth TailMode = iota
	TailModeStdout
	TailModeStderr
)

// TailKeyMap defines keybindings for the tail view
type TailKeyMap struct {
	Quit          key.Binding
	Pause         key.Binding
	Follow        key.Binding
	Clear         key.Binding
	Bottom        key.Binding
	Top           key.Binding
	ShowStdout    key.Binding
	ShowStderr    key.Binding
	ShowBoth      key.Binding
	NextPane      key.Binding
	ToggleLayout  key.Binding
	ToggleBorders key.Binding
	ToggleMouse   key.Binding
	Search        key.Binding
	FindNext      key.Binding
	FindPrev      key.Binding
	CopySelection key.Binding
	CopyMode      key.Binding
	ViewPager     key.Binding
	CopyAll       key.Binding
	ToggleHelp    key.Binding
}

func (k TailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.ShowStdout, k.ShowStderr, k.ShowBoth, k.Follow, k.Search, k.FindNext, k.FindPrev, k.CopySelection, k.CopyAll, k.ToggleHelp}
}

func (k TailKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.ShowStdout, k.ShowStderr, k.ShowBoth, k.NextPane, k.ToggleLayout, k.ToggleBorders, k.ToggleMouse, k.CopySelection, k.CopyMode, k.ViewPager, k.CopyAll, k.ToggleHelp},
		{k.Follow, k.Pause, k.Clear, k.Bottom, k.Search, k.FindNext, k.FindPrev, k.Quit},
	}
}

var tailKeys = TailKeyMap{
	Quit:          key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
	Pause:         key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause")),
	Follow:        key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "follow")),
	Clear:         key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
	Bottom:        key.NewBinding(key.WithKeys("b", "G"), key.WithHelp("b/G", "bottom")),
	Top:           key.NewBinding(key.WithKeys("t", "home", "g"), key.WithHelp("t/g", "top")),
	ShowStdout:    key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "stdout")),
	ShowStderr:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "stderr")),
	ShowBoth:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "both")),
	NextPane:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
	ToggleLayout:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "layout")),
	ToggleBorders: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "borders")),
	ToggleMouse:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mouse")),
	Search:        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	FindNext:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
	FindPrev:      key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
	CopySelection: key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^y", "copy sel")),
	CopyMode:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy mode")),
	ViewPager:     key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view in vim")),
	CopyAll:       key.NewBinding(key.WithKeys("Y"), key.WithHelp("Y", "copy pane")),
	ToggleHelp:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
}

type logLineMsg struct {
	pane     string // "stdout" or "stderr"
	line     string
	err      error
	terminal bool
}

type tailStartMsg struct {
	pane         string
	initialLines []string
	reader       *bufio.Reader
	cmd          *exec.Cmd
	pipe         *os.File
	startErr     error
}

// TailModel handles the dual-pane log viewing
type TailModel struct {
	jobID      string
	stdoutPath string
	stderrPath string

	mode TailMode // New field

	stdoutView viewport.Model
	stderrView viewport.Model

	stdoutLines []string
	stderrLines []string

	wrappedStdout []string
	wrappedStderr []string

	// Cached, incrementally-built viewport content for each pane. This avoids
	// re-joining all lines on every appended log line.
	//
	// TailModel is frequently copied by value (Bubble Tea model updates return
	// copies). strings.Builder panics if it's copied after first use, so we keep
	// pointers to builders to preserve their address across copies.
	stdoutBuilder *strings.Builder
	stderrBuilder *strings.Builder

	copyMode         bool
	prevMode         TailMode
	prevShowBorders  bool
	prevStacked      bool
	prevMouseEnabled bool
	prevActivePane   int

	// Readers for active streams
	stdoutReader *bufio.Reader
	stderrReader *bufio.Reader

	// Underlying pipe read ends (closed on cleanup).
	stdoutPipe *os.File
	stderrPipe *os.File

	// Keep commands alive
	stdoutCmd *exec.Cmd
	stderrCmd *exec.Cmd

	paused    bool
	following bool
	width     int
	height    int

	activePane   int  // 0: stdout, 1: stderr (for focus in split mode)
	stacked      bool // true: top/bottom, false: left/right
	showBorders  bool
	mouseEnabled bool

	// Search
	searchInput  textinput.Model
	inSearchMode bool
	lastSearch   string

	selectionPane   string
	selectionAnchor selectionPoint
	selectionCursor selectionPoint
	selecting       bool

	styles *TailStyles
}

type selectionPoint struct {
	line int
	col  int
}

type paneGeometry struct {
	x, y          int
	width, height int
	contentX      int
	contentY      int
	contentWidth  int
	contentHeight int
}

type TailStyles struct {
	Border lipgloss.Style
	Title  lipgloss.Style
}

func DefaultTailStyles() *TailStyles {
	return &TailStyles{
		Border: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.Border),
		Title:  lipgloss.NewStyle().Foreground(theme.TextStrong).Bold(true),
	}
}

func (m TailModel) InSearchMode() bool {
	return m.inSearchMode
}

// Helper for hidden border
var hiddenBorder = lipgloss.Border{
	Top:         " ",
	Bottom:      " ",
	Left:        " ",
	Right:       " ",
	TopLeft:     " ",
	TopRight:    " ",
	BottomRight: " ",
	BottomLeft:  " ",
}

const searchOverlayHeight = 4

var searchHighlightStyle = lipgloss.NewStyle().
	Background(theme.SearchBg).
	Foreground(theme.SearchFg).
	Bold(true).
	Padding(0, 1)

var tailSelectionStyle = lipgloss.NewStyle().
	Foreground(selectionFg).
	Background(selectionBg)

func NewTailModel(jobID, stdoutPath, stderrPath string, width, height int, mode TailMode) TailModel {
	m := TailModel{
		jobID:         jobID,
		stdoutPath:    stdoutPath,
		stderrPath:    stderrPath,
		mode:          mode,
		stdoutLines:   []string{},
		stderrLines:   []string{},
		wrappedStdout: []string{},
		wrappedStderr: []string{},
		stdoutBuilder: &strings.Builder{},
		stderrBuilder: &strings.Builder{},
		width:         width,
		height:        height,
		following:     true,
		showBorders:   true,
		styles:        DefaultTailStyles(),
	}

	// Search init
	ti := textinput.New()
	ti.Placeholder = "Type to search"
	ti.CharLimit = 156
	if width > 0 {
		searchWidth := width - 10
		if searchWidth < 20 {
			searchWidth = 20
		}
		ti.Width = searchWidth
	} else {
		ti.Width = 30
	}
	ti.Prompt = ""
	ti.PromptStyle = lipgloss.NewStyle()
	ti.TextStyle = lipgloss.NewStyle().Foreground(textStrong)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(subtle)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(highlight)
	m.searchInput = ti

	// Initialize viewports

	// Calculate widths based on mode
	var stdoutWidth, stderrWidth int

	vpHeight := height - 5
	if vpHeight < 5 {
		vpHeight = 5
	}

	if mode == TailModeBoth {
		avail := width - 4
		if avail < 20 {
			avail = 20
		}

		halfWidth := avail / 2
		stdoutWidth = halfWidth
		stderrWidth = avail - halfWidth
	} else {
		fullWidth := width - 2
		if fullWidth < 10 {
			fullWidth = 10
		}
		stdoutWidth = fullWidth
		stderrWidth = fullWidth
	}

	m.stdoutView = viewport.New(stdoutWidth, vpHeight)
	m.stdoutView.SetContent("Initializing stdout tail...")

	m.stderrView = viewport.New(stderrWidth, vpHeight)
	m.stderrView.SetContent("Initializing stderr tail...")

	m.recalculateLayout()

	return m
}

func (m *TailModel) recalculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// height - 5 to be safe (Title + Border + Buffer)
	vpHeight := m.height - 5
	if m.inSearchMode {
		vpHeight -= searchOverlayHeight
	}
	if vpHeight < 5 {
		vpHeight = 5
	}

	var stdoutWidth, stderrWidth int
	var stdoutHeight, stderrHeight int

	// Defaults
	stdoutHeight = vpHeight
	stderrHeight = vpHeight

	if m.mode == TailModeBoth {
		if m.stacked {
			// Top/Bottom split
			// Reduce width to avoid right edge overflow
			avail := m.width - 4
			if avail < 10 {
				avail = 10
			}

			stdoutWidth = avail
			stderrWidth = avail

			halfHeight := vpHeight / 2
			stdoutHeight = halfHeight
			stderrHeight = vpHeight - halfHeight // Give remainder to stderr
		} else {
			// Left/Right split
			// Each pane has 2 chars border. Total 4 chars reserved.
			// We add 2 more for safety margin against right edge.
			avail := m.width - 6
			if avail < 20 {
				avail = 20
			}

			halfWidth := avail / 2
			stdoutWidth = halfWidth
			stderrWidth = avail - halfWidth
		}
	} else {
		// Single mode
		avail := m.width - 4
		if avail < 10 {
			avail = 10
		}
		stdoutWidth = avail
		stderrWidth = avail
	}

	m.stdoutView.Width = stdoutWidth
	m.stdoutView.Height = stdoutHeight
	m.stderrView.Width = stderrWidth
	m.stderrView.Height = stderrHeight

	m.refreshViewportContent()

	searchWidth := m.width - 10
	if searchWidth < 20 {
		searchWidth = 20
	}
	m.searchInput.Width = searchWidth
}

func (m TailModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.mode == TailModeBoth || m.mode == TailModeStdout {
		cmds = append(cmds, m.startTailCmd("stdout", m.stdoutPath))
	}
	if m.mode == TailModeBoth || m.mode == TailModeStderr {
		cmds = append(cmds, m.startTailCmd("stderr", m.stderrPath))
	}
	return tea.Batch(cmds...)
}

var ansiCursorRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[A-KSTf]`)

// Helper to clean log lines (handle CR and ANSI cursor codes)
func cleanLogLine(line string) string {
	// 1. Remove ANSI cursor movement codes
	line = ansiCursorRegexp.ReplaceAllString(line, "")

	// 2. Handle CR
	// Trim trailing CR to avoid losing the line content if it ends with CR
	line = strings.TrimRight(line, "\r")
	if idx := strings.LastIndex(line, "\r"); idx != -1 {
		return line[idx+1:]
	}
	return line
}

func (m *TailModel) appendLogLine(pane string, lines *[]string, wrapped *[]string, b *strings.Builder, view *viewport.Model, text string) {
	cleanLine := cleanLogLine(text)
	*lines = append(*lines, cleanLine)
	if MaxLogLines > 0 && len(*lines) > MaxLogLines {
		*lines = (*lines)[1:]
	}

	wrappedLine := m.wrapLine(cleanLine, view.Width)
	*wrapped = append(*wrapped, wrappedLine)

	visualLinesRemoved := 0
	trimmedWrapped := false
	if MaxLogLines > 0 && len(*wrapped) > MaxLogLines {
		removedBlock := (*wrapped)[0]
		visualLinesRemoved = visualLineCount(removedBlock)
		*wrapped = (*wrapped)[1:]
		trimmedWrapped = true
	}
	m.adjustSelectionAfterTrim(pane, visualLinesRemoved)

	stickToBottom := m.following && !m.paused && view.AtBottom()

	needle := strings.ToLower(m.activeSearchTerm())
	if trimmedWrapped {
		// Can't efficiently remove from the front; rebuild.
		m.rebuildPaneContent(pane, b, *wrapped, needle)
	} else {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(renderLineForSearch(wrappedLine, needle))
	}
	view.SetContent(b.String())

	if stickToBottom {
		view.GotoBottom()
	} else if visualLinesRemoved > 0 && view.YOffset > 0 {
		view.YOffset -= visualLinesRemoved
		if view.YOffset < 0 {
			view.YOffset = 0
		}
	}
}

func (m *TailModel) wrapLine(line string, width int) string {
	if line == "" {
		return ""
	}
	if width <= 0 {
		return line
	}
	return wordwrap.String(line, width)
}

func (m *TailModel) activeSearchTerm() string {
	if m.inSearchMode {
		if val := strings.TrimSpace(m.searchInput.Value()); val != "" {
			return val
		}
	}
	return strings.TrimSpace(m.lastSearch)
}

func (m *TailModel) clearSelection() {
	m.selectionPane = ""
	m.selectionAnchor = selectionPoint{}
	m.selectionCursor = selectionPoint{}
	m.selecting = false
}

func (m TailModel) hasSelectionInPane(pane string) bool {
	return m.selectionPane == pane && (m.selecting || m.selectionAnchor != m.selectionCursor)
}

func normalizeSelection(a, b selectionPoint) (selectionPoint, selectionPoint) {
	if a.line > b.line || (a.line == b.line && a.col > b.col) {
		return b, a
	}
	return a, b
}

func flattenWrappedLines(wrapped []string) []string {
	if len(wrapped) == 0 {
		return nil
	}
	lines := make([]string, 0, len(wrapped))
	for _, block := range wrapped {
		lines = append(lines, strings.Split(block, "\n")...)
	}
	return lines
}

func visualLineCount(block string) int {
	return strings.Count(block, "\n") + 1
}

func runeLen(s string) int {
	return len([]rune(s))
}

func runeSlice(s string, start, end int) string {
	r := []rune(s)
	if start < 0 {
		start = 0
	}
	if end > len(r) {
		end = len(r)
	}
	if start > end {
		start = end
	}
	return string(r[start:end])
}

func (m TailModel) paneVisualLines(pane string) []string {
	switch pane {
	case "stdout":
		return flattenWrappedLines(m.wrappedStdout)
	case "stderr":
		return flattenWrappedLines(m.wrappedStderr)
	default:
		return nil
	}
}

func (m TailModel) paneGeometry(pane string) (paneGeometry, bool) {
	headerHeight := 1
	borderX := 2
	borderY := 2

	makeGeom := func(x, y, vpWidth, vpHeight int) paneGeometry {
		return paneGeometry{
			x:             x,
			y:             y,
			width:         vpWidth + borderX,
			height:        headerHeight + vpHeight + borderY,
			contentX:      x + 1,
			contentY:      y + headerHeight + 1,
			contentWidth:  vpWidth,
			contentHeight: vpHeight,
		}
	}

	switch m.mode {
	case TailModeStdout:
		if pane != "stdout" {
			return paneGeometry{}, false
		}
		return makeGeom(0, 0, m.stdoutView.Width, m.stdoutView.Height), true
	case TailModeStderr:
		if pane != "stderr" {
			return paneGeometry{}, false
		}
		return makeGeom(0, 0, m.stderrView.Width, m.stderrView.Height), true
	default:
		stdoutGeom := makeGeom(0, 0, m.stdoutView.Width, m.stdoutView.Height)
		stderrGeom := makeGeom(0, 0, m.stderrView.Width, m.stderrView.Height)
		if m.stacked {
			stderrGeom.y = stdoutGeom.height
			stderrGeom.contentY = stderrGeom.y + headerHeight + 1
		} else {
			stderrGeom.x = stdoutGeom.width
			stderrGeom.contentX = stderrGeom.x + 1
		}
		if pane == "stdout" {
			return stdoutGeom, true
		}
		if pane == "stderr" {
			return stderrGeom, true
		}
		return paneGeometry{}, false
	}
}

func (m TailModel) paneFromMouse(x, y int) string {
	switch m.mode {
	case TailModeStdout:
		if g, ok := m.paneGeometry("stdout"); ok {
			if x >= g.x && x < g.x+g.width && y >= g.y && y < g.y+g.height {
				return "stdout"
			}
		}
		return ""
	case TailModeStderr:
		if g, ok := m.paneGeometry("stderr"); ok {
			if x >= g.x && x < g.x+g.width && y >= g.y && y < g.y+g.height {
				return "stderr"
			}
		}
		return ""
	default:
		if g, ok := m.paneGeometry("stdout"); ok {
			if x >= g.x && x < g.x+g.width && y >= g.y && y < g.y+g.height {
				return "stdout"
			}
		}
		if g, ok := m.paneGeometry("stderr"); ok {
			if x >= g.x && x < g.x+g.width && y >= g.y && y < g.y+g.height {
				return "stderr"
			}
		}
		return ""
	}
}

func (m TailModel) paneSelectionPoint(pane string, x, y int, clampToViewport bool) (selectionPoint, bool) {
	geom, ok := m.paneGeometry(pane)
	if !ok || geom.contentWidth <= 0 || geom.contentHeight <= 0 {
		return selectionPoint{}, false
	}

	inPane := x >= geom.x && x < geom.x+geom.width && y >= geom.y && y < geom.y+geom.height
	if !inPane {
		return selectionPoint{}, false
	}

	localX := x - geom.contentX
	localY := y - geom.contentY

	if clampToViewport {
		if localX < 0 {
			localX = 0
		}
		if localX > geom.contentWidth {
			localX = geom.contentWidth
		}
		if localY < 0 {
			localY = 0
		}
		if localY >= geom.contentHeight {
			localY = geom.contentHeight - 1
		}
	} else if localX < 0 || localX >= geom.contentWidth || localY < 0 || localY >= geom.contentHeight {
		return selectionPoint{}, false
	}

	var vp viewport.Model
	switch pane {
	case "stdout":
		vp = m.stdoutView
	case "stderr":
		vp = m.stderrView
	default:
		return selectionPoint{}, false
	}

	lines := m.paneVisualLines(pane)
	line := vp.YOffset + localY
	if len(lines) == 0 {
		return selectionPoint{line: 0, col: 0}, true
	}
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}

	col := localX
	maxCol := runeLen(lines[line])
	if col < 0 {
		col = 0
	}
	if col > maxCol {
		col = maxCol
	}
	return selectionPoint{line: line, col: col}, true
}

func (m *TailModel) refreshPaneContent(pane string) {
	switch pane {
	case "stdout":
		m.refreshStdoutContent()
	case "stderr":
		m.refreshStderrContent()
	}
}

func isWheelUp(msg tea.MouseMsg) bool {
	return msg.Type == tea.MouseWheelUp ||
		(msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp)
}

func isWheelMouse(msg tea.MouseMsg) bool {
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown, tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight:
			return true
		}
	}
	return msg.Type == tea.MouseWheelUp ||
		msg.Type == tea.MouseWheelDown ||
		msg.Type == tea.MouseWheelLeft ||
		msg.Type == tea.MouseWheelRight
}

func (m *TailModel) adjustSelectionAfterTrim(pane string, removedVisualLines int) {
	if removedVisualLines <= 0 || m.selectionPane != pane {
		return
	}

	m.selectionAnchor.line -= removedVisualLines
	m.selectionCursor.line -= removedVisualLines

	if m.selectionAnchor.line < 0 && m.selectionCursor.line < 0 {
		m.clearSelection()
		return
	}
	if m.selectionAnchor.line < 0 {
		m.selectionAnchor.line = 0
	}
	if m.selectionCursor.line < 0 {
		m.selectionCursor.line = 0
	}
}

func (m TailModel) selectionBoundsForLine(pane string, lineIndex int, line string) (int, int, bool) {
	if m.selectionPane != pane {
		return 0, 0, false
	}

	start, end := normalizeSelection(m.selectionAnchor, m.selectionCursor)
	if lineIndex < start.line || lineIndex > end.line {
		return 0, 0, false
	}

	lineLen := runeLen(line)
	selStart := 0
	selEnd := lineLen
	if lineIndex == start.line {
		selStart = start.col
	}
	if lineIndex == end.line {
		selEnd = end.col
	}

	if selStart < 0 {
		selStart = 0
	}
	if selStart > lineLen {
		selStart = lineLen
	}
	if selEnd < 0 {
		selEnd = 0
	}
	if selEnd > lineLen {
		selEnd = lineLen
	}
	if selEnd < selStart {
		selStart, selEnd = selEnd, selStart
	}
	return selStart, selEnd, selEnd > selStart
}

func (m TailModel) selectedText() string {
	if m.selectionPane == "" {
		return ""
	}
	lines := m.paneVisualLines(m.selectionPane)
	if len(lines) == 0 {
		return ""
	}

	start, end := normalizeSelection(m.selectionAnchor, m.selectionCursor)
	if start.line < 0 {
		start.line = 0
	}
	if end.line >= len(lines) {
		end.line = len(lines) - 1
	}
	if start.line > end.line {
		return ""
	}

	var b strings.Builder
	for i := start.line; i <= end.line; i++ {
		line := lines[i]
		lineLen := runeLen(line)

		selStart := 0
		if i == start.line {
			selStart = start.col
		}
		selEnd := lineLen
		if i == end.line {
			selEnd = end.col
		}
		if selStart < 0 {
			selStart = 0
		}
		if selStart > lineLen {
			selStart = lineLen
		}
		if selEnd < 0 {
			selEnd = 0
		}
		if selEnd > lineLen {
			selEnd = lineLen
		}
		if selEnd < selStart {
			selStart, selEnd = selEnd, selStart
		}

		b.WriteString(runeSlice(line, selStart, selEnd))
		if i < end.line {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func renderLineForSearch(line, needle string) string {
	if needle == "" {
		return line
	}
	return highlightMatches(line, needle)
}

func renderDecoratedLine(line, needle string, selStart, selEnd int, selected bool) string {
	if !selected {
		return renderLineForSearch(line, needle)
	}
	prefix := runeSlice(line, 0, selStart)
	selection := runeSlice(line, selStart, selEnd)
	suffix := runeSlice(line, selEnd, runeLen(line))

	var b strings.Builder
	b.WriteString(renderLineForSearch(prefix, needle))
	b.WriteString(tailSelectionStyle.Render(renderLineForSearch(selection, needle)))
	b.WriteString(renderLineForSearch(suffix, needle))
	return b.String()
}

func (m *TailModel) rebuildPaneContent(pane string, b *strings.Builder, wrapped []string, needle string) {
	b.Reset()
	lineIndex := 0
	for blockIndex, block := range wrapped {
		lines := strings.Split(block, "\n")
		for i, line := range lines {
			if blockIndex > 0 || i > 0 {
				b.WriteByte('\n')
			}
			selStart, selEnd, selected := m.selectionBoundsForLine(pane, lineIndex, line)
			b.WriteString(renderDecoratedLine(line, needle, selStart, selEnd, selected))
			lineIndex++
		}
	}
}

func highlightMatches(line, needle string) string {
	if needle == "" || strings.TrimSpace(line) == "" {
		return line
	}

	lowerLine := strings.ToLower(line)
	var b strings.Builder
	i := 0

	for i < len(line) {
		idx := strings.Index(lowerLine[i:], needle)
		if idx == -1 {
			b.WriteString(line[i:])
			break
		}
		start := i + idx
		end := start + len(needle)
		b.WriteString(line[i:start])
		match := strings.ToUpper(line[start:end])
		b.WriteString(searchHighlightStyle.Render(match))
		i = end
	}
	return b.String()
}

func (m *TailModel) refreshViewportContent() {
	m.refreshStdoutContent()
	m.refreshStderrContent()
}

func (m *TailModel) refreshStdoutContent() {
	m.wrappedStdout = m.wrappedStdout[:0]
	for _, line := range m.stdoutLines {
		m.wrappedStdout = append(m.wrappedStdout, m.wrapLine(line, m.stdoutView.Width))
	}
	if m.stdoutBuilder == nil {
		m.stdoutBuilder = &strings.Builder{}
	}
	m.rebuildPaneContent("stdout", m.stdoutBuilder, m.wrappedStdout, strings.ToLower(m.activeSearchTerm()))
	m.stdoutView.SetContent(m.stdoutBuilder.String())
}

func (m *TailModel) refreshStderrContent() {
	m.wrappedStderr = m.wrappedStderr[:0]
	for _, line := range m.stderrLines {
		m.wrappedStderr = append(m.wrappedStderr, m.wrapLine(line, m.stderrView.Width))
	}
	if m.stderrBuilder == nil {
		m.stderrBuilder = &strings.Builder{}
	}
	m.rebuildPaneContent("stderr", m.stderrBuilder, m.wrappedStderr, strings.ToLower(m.activeSearchTerm()))
	m.stderrView.SetContent(m.stderrBuilder.String())
}

func (m *TailModel) enterCopyMode() tea.Cmd {
	if m.copyMode {
		return nil
	}
	m.clearSelection()
	m.copyMode = true
	m.following = false
	m.prevMode = m.mode
	m.prevShowBorders = m.showBorders
	m.prevStacked = m.stacked
	m.prevMouseEnabled = m.mouseEnabled
	m.prevActivePane = m.activePane

	if m.mode == TailModeBoth {
		if m.activePane == 1 {
			m.mode = TailModeStderr
		} else {
			m.mode = TailModeStdout
		}
	}

	m.showBorders = false
	m.recalculateLayout()

	if m.prevMouseEnabled {
		m.mouseEnabled = false
		return tea.DisableMouse
	}
	return nil
}

func (m *TailModel) exitCopyMode() tea.Cmd {
	if !m.copyMode {
		return nil
	}

	m.copyMode = false
	m.mode = m.prevMode
	m.showBorders = m.prevShowBorders
	m.stacked = m.prevStacked
	m.activePane = m.prevActivePane
	m.recalculateLayout()

	if m.prevMouseEnabled {
		m.mouseEnabled = true
		return tea.EnableMouseCellMotion
	}
	return nil
}

func (m *TailModel) openInPagerCmd(path string) tea.Cmd {
	if path == "" {
		return nil
	}

	pager := os.Getenv("PAGER")
	var cmd *exec.Cmd

	if pager != "" {
		fields := strings.Fields(pager)
		if len(fields) > 0 {
			bin := fields[0]
			args := []string{}
			if len(fields) > 1 {
				args = append(args, fields[1:]...)
			}
			args = append(args, path)
			cmd = exec.Command(bin, args...)
		}
	}

	if cmd == nil {
		cmd = exec.Command("vim", "-R", path)
	}

	return tea.ExecProcess(cmd, nil)
}

func (m TailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	if m.inSearchMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.inSearchMode = false
				m.lastSearch = m.searchInput.Value()
				m.performSearch(m.lastSearch, true)
				m.refreshViewportContent()
				m.searchInput.Blur()
				m.recalculateLayout()
				return m, nil
			case "esc":
				m.inSearchMode = false
				m.searchInput.Blur()
				m.refreshViewportContent()
				m.recalculateLayout()
				return m, nil
			}
		}
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
		m.refreshViewportContent()
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Guard against transient zero-size events by reusing the last known
		// size or a sensible default instead of ignoring the resize entirely.
		width := msg.Width
		height := msg.Height
		if width <= 0 {
			if m.width > 0 {
				width = m.width
			} else {
				width = 80
			}
		}
		if height <= 0 {
			if m.height > 0 {
				height = m.height
			} else {
				height = 24
			}
		}

		if width != m.width || height != m.height {
			m.clearSelection()
		}
		m.width = width
		m.height = height
		m.recalculateLayout()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, tailKeys.Quit):
			// Cleanup tail subprocesses and close pipes. Return no message; parent
			// handles switching back to the main view.
			stdoutCmd := m.stdoutCmd
			stderrCmd := m.stderrCmd
			stdoutPipe := m.stdoutPipe
			stderrPipe := m.stderrPipe

			m.stdoutCmd = nil
			m.stderrCmd = nil
			m.stdoutReader = nil
			m.stderrReader = nil
			m.stdoutPipe = nil
			m.stderrPipe = nil

			return m, tea.Batch(
				cleanupProcessCmd(stdoutCmd, stdoutPipe),
				cleanupProcessCmd(stderrCmd, stderrPipe),
			)
		case key.Matches(msg, tailKeys.Pause):
			m.paused = !m.paused
		case key.Matches(msg, tailKeys.Follow):
			m.following = !m.following
			if m.following {
				m.stdoutView.GotoBottom()
				m.stderrView.GotoBottom()
			}
		case key.Matches(msg, tailKeys.Clear):
			m.stdoutLines = []string{}
			m.stderrLines = []string{}
			m.wrappedStdout = []string{}
			m.wrappedStderr = []string{}
			m.clearSelection()
			if m.stdoutBuilder != nil {
				m.stdoutBuilder.Reset()
			}
			if m.stderrBuilder != nil {
				m.stderrBuilder.Reset()
			}
			m.stdoutView.SetContent("")
			m.stderrView.SetContent("")
		case key.Matches(msg, tailKeys.Bottom):
			m.following = true
			// Consume this key: the underlying viewport uses "b" for PageUp.
			// If we fall through to viewport.Update, we'd jump to bottom and then
			// immediately page up (making "b" feel like it doesn't reach bottom).
			// In Both mode, apply to the active pane only.
			switch m.mode {
			case TailModeStdout:
				m.stdoutView.GotoBottom()
			case TailModeStderr:
				m.stderrView.GotoBottom()
			default: // TailModeBoth
				if m.activePane == 0 {
					m.stdoutView.GotoBottom()
				} else {
					m.stderrView.GotoBottom()
				}
			}
			return m, tea.Batch(cmds...)
		case key.Matches(msg, tailKeys.Top):
			m.following = false
			// In Both mode, apply to the active pane only.
			switch m.mode {
			case TailModeStdout:
				m.stdoutView.GotoTop()
			case TailModeStderr:
				m.stderrView.GotoTop()
			default: // TailModeBoth
				if m.activePane == 0 {
					m.stdoutView.GotoTop()
				} else {
					m.stderrView.GotoTop()
				}
			}
			return m, tea.Batch(cmds...)
		case key.Matches(msg, tailKeys.CopyMode):
			var copyCmd tea.Cmd
			if m.copyMode {
				copyCmd = m.exitCopyMode()
			} else {
				copyCmd = m.enterCopyMode()
			}
			if copyCmd != nil {
				cmds = append(cmds, copyCmd)
			}
		case key.Matches(msg, tailKeys.ShowStdout):
			m.mode = TailModeStdout
			if m.mouseEnabled {
				m.mouseEnabled = false // Auto-disable mouse for easier copying
				cmds = append(cmds, tea.DisableMouse)
			}
			m.recalculateLayout()
		case key.Matches(msg, tailKeys.ShowStderr):
			m.mode = TailModeStderr
			if m.mouseEnabled {
				m.mouseEnabled = false // Auto-disable mouse for easier copying
				cmds = append(cmds, tea.DisableMouse)
			}
			m.recalculateLayout()
		case key.Matches(msg, tailKeys.ShowBoth):
			if m.copyMode {
				if copyCmd := m.exitCopyMode(); copyCmd != nil {
					cmds = append(cmds, copyCmd)
				}
			}
			m.mode = TailModeBoth
			// Optional: Restore mouse? Let's leave it to manual toggle or user preference
			// m.mouseEnabled = true
			// cmds = append(cmds, tea.EnableMouseCellMotion)
			m.recalculateLayout()
		case key.Matches(msg, tailKeys.NextPane):
			if m.mode == TailModeBoth {
				m.activePane = (m.activePane + 1) % 2
			}
		case key.Matches(msg, tailKeys.ToggleLayout):
			if m.copyMode && m.mode != TailModeBoth {
				break
			}
			m.stacked = !m.stacked
			m.recalculateLayout()
		case key.Matches(msg, tailKeys.ToggleBorders):
			if m.copyMode {
				break
			}
			m.showBorders = !m.showBorders
		case key.Matches(msg, tailKeys.ToggleMouse):
			m.mouseEnabled = !m.mouseEnabled
		case key.Matches(msg, tailKeys.Search):
			m.inSearchMode = true
			if focusCmd := m.searchInput.Focus(); focusCmd != nil {
				cmds = append(cmds, focusCmd)
			}
			m.searchInput.SetValue("")
			m.recalculateLayout()
			return m, tea.Batch(cmds...)
		case key.Matches(msg, tailKeys.FindNext):
			if m.lastSearch != "" {
				m.performSearch(m.lastSearch, true)
			}
		case key.Matches(msg, tailKeys.FindPrev):
			if m.lastSearch != "" {
				m.performSearch(m.lastSearch, false)
			}
		case key.Matches(msg, tailKeys.CopySelection):
			if selected := m.selectedText(); selected != "" {
				cmds = append(cmds, osc52CopyCmd(selected))
			}
			return m, tea.Batch(cmds...)
		case key.Matches(msg, tailKeys.CopyAll):
			var lines []string
			switch m.mode {
			case TailModeStdout:
				lines = m.stdoutLines
			case TailModeStderr:
				lines = m.stderrLines
			case TailModeBoth:
				if m.activePane == 1 {
					lines = m.stderrLines
				} else {
					lines = m.stdoutLines
				}
			}
			if len(lines) > 0 {
				text := strings.Join(lines, "\n")
				cmds = append(cmds, osc52CopyCmd(text))
			}
			// Don't fall through to viewport.Update for this key
			return m, tea.Batch(cmds...)
		case key.Matches(msg, tailKeys.ViewPager):
			var path string
			switch m.mode {
			case TailModeStdout:
				path = m.stdoutPath
			case TailModeStderr:
				path = m.stderrPath
			case TailModeBoth:
				if m.activePane == 1 {
					path = m.stderrPath
				} else {
					path = m.stdoutPath
				}
			}
			if path != "" {
				if pagerCmd := m.openInPagerCmd(path); pagerCmd != nil {
					cmds = append(cmds, pagerCmd)
				}
			}
		}

		// Detect scrolling keys to disable follow
		// viewport keys: k, j, up, down, pgup, pgdown
		// We can just check the string
		k := msg.String()
		if k == "up" || k == "k" || k == "pgup" || k == "wheel up" {
			m.following = false
		}
		// If we scroll down and hit bottom, maybe re-enable?
		// For now, let's just disable on scroll up.

		// Dispatch to appropriate viewport
		if m.mode == TailModeStdout {
			m.stdoutView, cmd = m.stdoutView.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.mode == TailModeStderr {
			m.stderrView, cmd = m.stderrView.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			// In Both mode, send to active pane only for scrolling/interactions
			if m.activePane == 0 {
				m.stdoutView, cmd = m.stdoutView.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				m.stderrView, cmd = m.stderrView.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case tea.MouseMsg:
		pane := m.paneFromMouse(msg.X, msg.Y)
		if pane == "stdout" {
			m.activePane = 0
		} else if pane == "stderr" {
			m.activePane = 1
		}

		if isWheelUp(msg) {
			m.following = false
		}

		if isWheelMouse(msg) {
			targetPane := pane
			if targetPane == "" {
				if m.mode == TailModeStdout {
					targetPane = "stdout"
				} else if m.mode == TailModeStderr {
					targetPane = "stderr"
				} else if m.activePane == 1 {
					targetPane = "stderr"
				} else {
					targetPane = "stdout"
				}
			}

			if targetPane == "stdout" {
				m.stdoutView, cmd = m.stdoutView.Update(msg)
				cmds = append(cmds, cmd)
			} else if targetPane == "stderr" {
				m.stderrView, cmd = m.stderrView.Update(msg)
				cmds = append(cmds, cmd)
			}

			if m.selecting && m.selectionPane != "" {
				if pt, ok := m.paneSelectionPoint(m.selectionPane, msg.X, msg.Y, true); ok {
					if pt != m.selectionCursor {
						m.selectionCursor = pt
						m.refreshPaneContent(m.selectionPane)
					}
				}
			}
			break
		}

		leftPress := msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonLeft || msg.Type == tea.MouseLeft)
		leftMotion := msg.Action == tea.MouseActionMotion && (msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone || msg.Type == tea.MouseMotion)
		leftRelease := msg.Action == tea.MouseActionRelease || msg.Type == tea.MouseRelease

		switch {
		case leftPress:
			if pane == "" {
				break
			}
			pt, ok := m.paneSelectionPoint(pane, msg.X, msg.Y, false)
			if !ok {
				break
			}
			m.following = false
			m.selectionPane = pane
			m.selectionAnchor = pt
			m.selectionCursor = pt
			m.selecting = true
			m.refreshPaneContent(pane)
		case leftMotion && m.selecting:
			if m.selectionPane == "" {
				break
			}
			pt, ok := m.paneSelectionPoint(m.selectionPane, msg.X, msg.Y, true)
			if !ok {
				break
			}
			if pt != m.selectionCursor {
				m.selectionCursor = pt
				m.refreshPaneContent(m.selectionPane)
			}
		case leftRelease && m.selecting:
			if m.selectionPane != "" {
				if pt, ok := m.paneSelectionPoint(m.selectionPane, msg.X, msg.Y, true); ok {
					m.selectionCursor = pt
				}
				m.refreshPaneContent(m.selectionPane)
			}
			m.selecting = false
		}

	case tailStartMsg:
		// Set initial content in one shot to avoid visible "scrolling down" when
		// loading a lot of historical lines.
		if msg.pane == "stdout" {
			m.stdoutLines = m.stdoutLines[:0]
			for _, line := range msg.initialLines {
				m.stdoutLines = append(m.stdoutLines, cleanLogLine(line))
			}
			if MaxLogLines > 0 && len(m.stdoutLines) > MaxLogLines {
				m.stdoutLines = m.stdoutLines[len(m.stdoutLines)-MaxLogLines:]
			}
			m.refreshStdoutContent()
			if m.following && !m.paused {
				m.stdoutView.GotoBottom()
			}

			if msg.startErr != nil {
				// Error already explained in initialLines; don't start reader
				break
			}

			m.stdoutReader = msg.reader
			m.stdoutCmd = msg.cmd
			m.stdoutPipe = msg.pipe
			cmds = append(cmds, m.waitForLine("stdout", m.stdoutReader))
		} else {
			m.stderrLines = m.stderrLines[:0]
			for _, line := range msg.initialLines {
				m.stderrLines = append(m.stderrLines, cleanLogLine(line))
			}
			if MaxLogLines > 0 && len(m.stderrLines) > MaxLogLines {
				m.stderrLines = m.stderrLines[len(m.stderrLines)-MaxLogLines:]
			}
			m.refreshStderrContent()
			if m.following && !m.paused {
				m.stderrView.GotoBottom()
			}

			if msg.startErr != nil {
				// Error already explained in initialLines; don't start reader
				break
			}

			m.stderrReader = msg.reader
			m.stderrCmd = msg.cmd
			m.stderrPipe = msg.pipe
			cmds = append(cmds, m.waitForLine("stderr", m.stderrReader))
		}

	case logLineMsg:
		lineHasContent := msg.err == nil || msg.line != ""

		if msg.pane == "stdout" {
			if lineHasContent {
				if m.stdoutBuilder == nil {
					m.stdoutBuilder = &strings.Builder{}
				}
				m.appendLogLine("stdout", &m.stdoutLines, &m.wrappedStdout, m.stdoutBuilder, &m.stdoutView, msg.line)
			}

			if msg.err != nil {
				errLine := "EOF (tail exited)"
				if msg.err != io.EOF {
					errLine = fmt.Sprintf("Error reading: %v", msg.err)
				}
				if m.stdoutBuilder == nil {
					m.stdoutBuilder = &strings.Builder{}
				}
				m.appendLogLine("stdout", &m.stdoutLines, &m.wrappedStdout, m.stdoutBuilder, &m.stdoutView, errLine)
			}

			if !msg.terminal {
				cmds = append(cmds, m.waitForLine("stdout", m.stdoutReader))
			} else {
				// Ensure resources are released if the tail process exits.
				stdoutCmd := m.stdoutCmd
				stdoutPipe := m.stdoutPipe
				m.stdoutCmd = nil
				m.stdoutReader = nil
				m.stdoutPipe = nil
				cmds = append(cmds, cleanupProcessCmd(stdoutCmd, stdoutPipe))
			}
		} else {
			if lineHasContent {
				if m.stderrBuilder == nil {
					m.stderrBuilder = &strings.Builder{}
				}
				m.appendLogLine("stderr", &m.stderrLines, &m.wrappedStderr, m.stderrBuilder, &m.stderrView, msg.line)
			}

			if msg.err != nil {
				errLine := "EOF (tail exited)"
				if msg.err != io.EOF {
					errLine = fmt.Sprintf("Error reading: %v", msg.err)
				}
				if m.stderrBuilder == nil {
					m.stderrBuilder = &strings.Builder{}
				}
				m.appendLogLine("stderr", &m.stderrLines, &m.wrappedStderr, m.stderrBuilder, &m.stderrView, errLine)
			}

			if !msg.terminal {
				cmds = append(cmds, m.waitForLine("stderr", m.stderrReader))
			} else {
				stderrCmd := m.stderrCmd
				stderrPipe := m.stderrPipe
				m.stderrCmd = nil
				m.stderrReader = nil
				m.stderrPipe = nil
				cmds = append(cmds, cleanupProcessCmd(stderrCmd, stderrPipe))
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *TailModel) performSearch(query string, forward bool) {
	if query == "" {
		return
	}
	query = strings.ToLower(query) // Case insensitive for now

	// Helper to get active state
	var lines []string
	var vp *viewport.Model

	// If in both mode, search active pane
	// If single mode, search that mode

	if m.mode == TailModeStdout {
		lines = m.wrappedStdout
		vp = &m.stdoutView
	} else if m.mode == TailModeStderr {
		lines = m.wrappedStderr
		vp = &m.stderrView
	} else {
		if m.activePane == 0 {
			lines = m.wrappedStdout
			vp = &m.stdoutView
		} else {
			lines = m.wrappedStderr
			vp = &m.stderrView
		}
	}

	if len(lines) == 0 {
		return
	}

	currentY := vp.YOffset
	foundIndex := -1

	if forward {
		// Start from next line
		start := currentY + 1
		if start >= len(lines) {
			start = 0
		} // wrap around? or stop? let's wrap

		for i := start; i < len(lines); i++ {
			if strings.Contains(strings.ToLower(lines[i]), query) {
				foundIndex = i
				break
			}
		}
		// Wrap around
		if foundIndex == -1 {
			for i := 0; i < start; i++ {
				if strings.Contains(strings.ToLower(lines[i]), query) {
					foundIndex = i
					break
				}
			}
		}
	} else {
		// Backward
		start := currentY - 1
		if start < 0 {
			start = len(lines) - 1
		}

		for i := start; i >= 0; i-- {
			if strings.Contains(strings.ToLower(lines[i]), query) {
				foundIndex = i
				break
			}
		}
		// Wrap around
		if foundIndex == -1 {
			for i := len(lines) - 1; i > start; i-- {
				if strings.Contains(strings.ToLower(lines[i]), query) {
					foundIndex = i
					break
				}
			}
		}
	}

	if foundIndex != -1 {
		m.following = false // Disable follow on jump
		vp.YOffset = foundIndex

		// Persist changes to the struct copies?
		// NOTE: m.stdoutView is a Value type in the struct, so updating 'vp' pointer
		// updates the field because we took address &m.stdoutView
		// Wait, m is receiver (m *TailModel), so it works.
	}
}

func (m TailModel) View() string {
	if m.copyMode {
		var content string
		switch m.mode {
		case TailModeStderr:
			content = m.stderrView.View()
		case TailModeBoth:
			if m.activePane == 1 {
				content = m.stderrView.View()
			} else {
				content = m.stdoutView.View()
			}
		default:
			content = m.stdoutView.View()
		}

		if m.inSearchMode {
			return m.renderSearchOverlay(content)
		}
		return content
	}

	var stdoutStyle, stderrStyle lipgloss.Style

	// Determine styles based on active pane
	var baseBorder lipgloss.Style
	if m.showBorders {
		baseBorder = m.styles.Border.Copy()
	} else {
		baseBorder = m.styles.Border.Copy().Border(hiddenBorder)
	}

	activeBorder := highlight
	inactiveBorder := panelBorder

	if m.mode == TailModeBoth {
		if m.activePane == 0 {
			stdoutStyle = baseBorder.Copy().BorderForeground(activeBorder)
			stderrStyle = baseBorder.Copy().BorderForeground(inactiveBorder)
		} else {
			stdoutStyle = baseBorder.Copy().BorderForeground(inactiveBorder)
			stderrStyle = baseBorder.Copy().BorderForeground(activeBorder)
		}
	} else {
		// Single mode, always active color
		stdoutStyle = baseBorder.Copy().BorderForeground(activeBorder)
		stderrStyle = baseBorder.Copy().BorderForeground(activeBorder)
	}

	// Helper to generate headers
	header := func(name, pane, path string, vp viewport.Model, isActive bool) string {
		percent := vp.ScrollPercent()
		scroll := fmt.Sprintf("%.0f%%", percent*100)
		if vp.AtTop() {
			scroll = "Top"
		} else if vp.AtBottom() {
			scroll = "Bot"
		}

		status := ""
		if m.paused {
			status = " [PAUSED]"
		} else if m.following {
			status = " [FOLLOW]"
		}

		if m.mouseEnabled {
			status += " [MOUSE]"
		}
		if m.hasSelectionInPane(pane) {
			status += " [SEL]"
		}

		// Add active indicator
		prefix := "  "
		if isActive {
			prefix = "> "
		}
		name = prefix + name

		// Calculate available width
		// We want the header to fit within the viewport's width
		// Let's use vp.Width as the target max width for the title line.
		maxWidth := vp.Width
		if maxWidth < 20 {
			maxWidth = 20
		} // Safety

		// Fixed parts: "NAME  (SCROLL)STATUS"
		// We need a separator space after name and before parens
		fixedLen := lipgloss.Width(fmt.Sprintf("%s  (%s)%s", name, scroll, status))

		available := maxWidth - fixedLen

		displayPath := path
		if available < 3 {
			displayPath = ""
		} else if lipgloss.Width(path) > available {
			// Truncate from start
			// Use runes for correct slicing
			r := []rune(path)
			trim := len(r) - available + 1 // +1 for ellipsis
			if trim > 0 && trim < len(r) {
				displayPath = "…" + string(r[trim:])
			} else {
				// Fallback if calculation off
				if len(path) >= available {
					displayPath = path[len(path)-available+1:]
				}
			}
		}

		// Style the header line
		headerStyle := m.styles.Title.Copy()
		if !isActive {
			headerStyle = headerStyle.Foreground(theme.TextDim)
		}

		return headerStyle.Render(fmt.Sprintf("%s %s (%s)%s", name, displayPath, scroll, status))
	}

	wrapIfSearch := func(content string) string {
		if m.inSearchMode {
			return m.renderSearchOverlay(content)
		}
		return content
	}

	if m.mode == TailModeStdout {
		content := lipgloss.JoinVertical(lipgloss.Left,
			header("STDOUT", "stdout", m.stdoutPath, m.stdoutView, true),
			stdoutStyle.Render(m.stdoutView.View()),
		)
		return wrapIfSearch(content)
	}

	if m.mode == TailModeStderr {
		content := lipgloss.JoinVertical(lipgloss.Left,
			header("STDERR", "stderr", m.stderrPath, m.stderrView, true),
			stderrStyle.Render(m.stderrView.View()),
		)
		return wrapIfSearch(content)
	}

	// Determine active states for dual view
	stdoutActive := m.activePane == 0
	stderrActive := m.activePane == 1

	left := lipgloss.JoinVertical(lipgloss.Left,
		header("STDOUT", "stdout", m.stdoutPath, m.stdoutView, stdoutActive),
		stdoutStyle.Render(m.stdoutView.View()),
	)

	right := lipgloss.JoinVertical(lipgloss.Left,
		header("STDERR", "stderr", m.stderrPath, m.stderrView, stderrActive),
		stderrStyle.Render(m.stderrView.View()),
	)

	if m.stacked {
		return wrapIfSearch(lipgloss.JoinVertical(lipgloss.Left, left, right))
	}

	return wrapIfSearch(lipgloss.JoinHorizontal(lipgloss.Top, left, right))
}

func (m TailModel) renderSearchOverlay(content string) string {
	rawValue := m.searchInput.Value()
	displayValue := strings.TrimSpace(rawValue)
	if displayValue == "" {
		displayValue = "(type to search)"
	}
	if m.searchInput.Focused() {
		displayValue += " ▍"
	}

	builder := &strings.Builder{}
	builder.WriteString("\n/ Search: ")
	builder.WriteString(displayValue)
	builder.WriteString("\n")
	builder.WriteString("Press Enter to jump, Esc to cancel")
	builder.WriteString("\n\n")
	builder.WriteString(content)

	return builder.String()
}

// Commands

func (m *TailModel) startTailCmd(pane, path string) tea.Cmd {
	return func() tea.Msg {
		if path == "" {
			archiveDir := logArchiveDir()
			archiveHint := "  • No archived log found in the convention directory"
			if archiveDir != "" {
				archiveHint = fmt.Sprintf("  • No archived log found in %s", archiveDir)
			}
			return tailStartMsg{
				pane: pane,
				initialLines: []string{
					"⚠ No log path available",
					"",
					"This can happen when:",
					"  • Job is too old (purged from sacct)",
					"  • scontrol/sacct couldn't resolve paths",
					archiveHint,
					"  • Job was submitted without output files",
					"",
					"Convention for finished jobs:",
					"  • ~/.slurm-dashboard/logs/<jobid>.out",
					"  • ~/.slurm-dashboard/logs/<jobid>.err",
					"  • Override dir with SLURM_DASHBOARD_LOG_ARCHIVE_DIR",
				},
				startErr: fmt.Errorf("no path provided"),
			}
		}

		// Two-phase startup:
		//  1) Load initial history with `tail -n <N>` in one shot.
		//  2) Start follow with `tail -n 0 -F` so we don't replay history line-by-line.
		//
		// This avoids the UI visibly "scrolling down" when opening very long logs.
		linesArg := "+1"
		if MaxLogLines > 0 {
			linesArg = strconv.Itoa(MaxLogLines)
		}

		var initialLines []string
		if out, err := exec.Command("tail", "-n", linesArg, path).CombinedOutput(); err == nil {
			initialLines = splitTailOutput(out)
			if len(initialLines) == 0 {
				initialLines = []string{"(file exists but is empty)"}
			}
		} else {
			// File might not exist yet (job pending/starting) or be inaccessible
			initialLines = splitTailOutput(out)
			if len(initialLines) == 0 {
				initialLines = []string{
					fmt.Sprintf("⚠ Cannot read: %s", path),
					"",
					fmt.Sprintf("Error: %v", err),
					"",
					"Waiting for file to appear (tail -F)...",
				}
			}
		}

		cmd := exec.Command("tail", "-n", "0", "-F", path)

		// Create a pipe to capture both stdout and stderr
		r, w, err := os.Pipe()
		if err != nil {
			return tailStartMsg{pane: pane, initialLines: initialLines, startErr: fmt.Errorf("creating pipe: %w", err)}
		}

		cmd.Stdout = w
		cmd.Stderr = w

		if err := cmd.Start(); err != nil {
			w.Close()
			r.Close()
			return tailStartMsg{pane: pane, initialLines: initialLines, startErr: err}
		}

		// Close write end in parent so that when child closes it (on exit), scanner sees EOF
		w.Close()

		reader := bufio.NewReader(r)
		// We need to pass this reader back to the model to loop on it

		// Also, we need to keep the process reference somewhere if we want to kill it.
		// Ideally, we wrap this in a struct that we pass back.

		return tailStartMsg{pane: pane, initialLines: initialLines, reader: reader, cmd: cmd, pipe: r}
	}
}

func splitTailOutput(out []byte) []string {
	s := strings.TrimRight(string(out), "\r\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func (m *TailModel) waitForLine(pane string, reader *bufio.Reader) tea.Cmd {
	return func() tea.Msg {
		if reader == nil {
			return logLineMsg{pane: pane, err: fmt.Errorf("log reader not initialized"), terminal: true}
		}

		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")

		return logLineMsg{pane: pane, line: line, err: err, terminal: err != nil}
	}
}

func osc52CopyCmd(text string) tea.Cmd {
	return func() tea.Msg {
		seq := osc52.New(text).Limit(100 * 1024)

		term := strings.ToLower(os.Getenv("TERM"))
		if tmux := os.Getenv("TMUX"); tmux != "" || strings.HasPrefix(term, "tmux") {
			seq = seq.Tmux()
		} else if strings.HasPrefix(term, "screen") {
			seq = seq.Screen()
		}

		_, _ = seq.WriteTo(os.Stdout)
		return nil
	}
}

func cleanupProcessCmd(cmd *exec.Cmd, pipe *os.File) tea.Cmd {
	return func() tea.Msg {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		if pipe != nil {
			_ = pipe.Close()
		}
		return nil
	}
}
