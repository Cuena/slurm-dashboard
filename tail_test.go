package main

import (
	"fmt"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func runTeaCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, subcmd := range batch {
			msgs = append(msgs, runTeaCmd(subcmd)...)
		}
		return msgs
	}

	return []tea.Msg{msg}
}

func TestTailBottomInBothModeAffectsOnlyActivePaneAndDoesNotPageUp(t *testing.T) {
	m := NewTailModel("1", "", "", 80, 12, TailModeBoth)
	m.mode = TailModeBoth
	m.activePane = 0

	m.stdoutLines = m.stdoutLines[:0]
	m.stderrLines = m.stderrLines[:0]
	for i := 0; i < 200; i++ {
		m.stdoutLines = append(m.stdoutLines, fmt.Sprintf("stdout line %d", i))
		m.stderrLines = append(m.stderrLines, fmt.Sprintf("stderr line %d", i))
	}
	m.refreshViewportContent()

	// Put the inactive pane somewhere mid-buffer so we can verify it doesn't move.
	m.stderrView.YOffset = 7

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	updated := model.(TailModel)

	if !updated.stdoutView.AtBottom() {
		t.Fatalf("expected active stdout pane to be at bottom after 'b', got YOffset=%d", updated.stdoutView.YOffset)
	}
	if updated.stderrView.YOffset != 7 {
		t.Fatalf("expected inactive stderr pane YOffset to remain unchanged, got %d", updated.stderrView.YOffset)
	}
}

func TestTailTopInBothModeAffectsOnlyActivePane(t *testing.T) {
	m := NewTailModel("1", "", "", 80, 12, TailModeBoth)
	m.mode = TailModeBoth
	m.activePane = 1

	m.stdoutLines = m.stdoutLines[:0]
	m.stderrLines = m.stderrLines[:0]
	for i := 0; i < 200; i++ {
		m.stdoutLines = append(m.stdoutLines, fmt.Sprintf("stdout line %d", i))
		m.stderrLines = append(m.stderrLines, fmt.Sprintf("stderr line %d", i))
	}
	m.refreshViewportContent()

	// Put the inactive pane somewhere non-zero so we can verify it doesn't move.
	m.stdoutView.YOffset = 9
	m.stderrView.YOffset = 11

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	updated := model.(TailModel)

	if updated.stderrView.YOffset != 0 {
		t.Fatalf("expected active stderr pane to be at top after 't', got YOffset=%d", updated.stderrView.YOffset)
	}
	if updated.stdoutView.YOffset != 9 {
		t.Fatalf("expected inactive stdout pane YOffset to remain unchanged, got %d", updated.stdoutView.YOffset)
	}
}

func TestTailSelectedTextAcrossOffscreenRange(t *testing.T) {
	m := NewTailModel("1", "", "", 80, 14, TailModeStdout)
	for i := 0; i < 50; i++ {
		m.stdoutLines = append(m.stdoutLines, fmt.Sprintf("line-%02d-value", i))
	}
	m.refreshStdoutContent()

	m.selectionPane = "stdout"
	m.selectionAnchor = selectionPoint{line: 3, col: 2}
	m.selectionCursor = selectionPoint{line: 12, col: 6}

	got := m.selectedText()
	lines := strings.Split(got, "\n")
	if len(lines) != 10 {
		t.Fatalf("expected 10 selected lines, got %d", len(lines))
	}
	if lines[0] != "ne-03-value" {
		t.Fatalf("unexpected first selected line: %q", lines[0])
	}
	if lines[len(lines)-1] != "line-1" {
		t.Fatalf("unexpected last selected line: %q", lines[len(lines)-1])
	}
}

func TestTailCopyModeUsesNativeTerminalSelection(t *testing.T) {
	m := NewTailModel("1", "", "", 80, 12, TailModeStdout)
	if m.mouseEnabled {
		t.Fatalf("expected mouse to start disabled")
	}

	enterCmd := m.enterCopyMode()
	if !m.copyMode {
		t.Fatalf("expected copy mode to be active")
	}
	if m.mouseEnabled {
		t.Fatalf("expected copy mode to keep mouse disabled for native terminal selection")
	}
	if enterCmd != nil {
		t.Fatalf("expected no mouse command when entering copy mode from mouse-off state")
	}

	exitCmd := m.exitCopyMode()
	if m.copyMode {
		t.Fatalf("expected copy mode to be cleared")
	}
	if m.mouseEnabled {
		t.Fatalf("expected mouse state to be restored after exiting copy mode")
	}
	if exitCmd != nil {
		t.Fatalf("expected no mouse command when exiting copy mode to mouse-off state")
	}

	m.mouseEnabled = true
	enterCmd = m.enterCopyMode()
	if m.mouseEnabled {
		t.Fatalf("expected copy mode to disable mouse when it was previously enabled")
	}
	if enterCmd == nil {
		t.Fatalf("expected disable-mouse command when entering copy mode from mouse-on state")
	}

	exitCmd = m.exitCopyMode()
	if !m.mouseEnabled {
		t.Fatalf("expected copy mode exit to restore previous mouse-on state")
	}
	if exitCmd == nil {
		t.Fatalf("expected enable-mouse command when restoring previous mouse-on state")
	}
}

func TestTailCopyModeIgnoresMouseSelectionEvents(t *testing.T) {
	m := NewTailModel("1", "", "", 80, 12, TailModeStdout)
	for i := 0; i < 40; i++ {
		m.stdoutLines = append(m.stdoutLines, fmt.Sprintf("line-%03d payload", i))
	}
	m.refreshViewportContent()
	m.enterCopyMode()

	model, _ := m.Update(tea.MouseMsg{
		X:      6,
		Y:      4,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		Type:   tea.MouseLeft,
	})
	m = model.(TailModel)

	if m.selecting || m.selectionPane != "" {
		t.Fatalf("expected copy mode to ignore in-app mouse selection, selecting=%v pane=%q", m.selecting, m.selectionPane)
	}
}

func TestTailMouseWheelExtendsSelectionWhileDragging(t *testing.T) {
	m := NewTailModel("1", "", "", 90, 20, TailModeStdout)
	for i := 0; i < 120; i++ {
		m.stdoutLines = append(m.stdoutLines, fmt.Sprintf("line-%03d payload", i))
	}
	m.refreshStdoutContent()

	startX, startY := 1, 3
	model, _ := m.Update(tea.MouseMsg{
		X:      startX,
		Y:      startY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		Type:   tea.MouseLeft,
	})
	m = model.(TailModel)

	for i := 0; i < 8; i++ {
		model, _ = m.Update(tea.MouseMsg{
			X:      startX,
			Y:      startY,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
			Type:   tea.MouseWheelDown,
		})
		m = model.(TailModel)
	}

	if !m.selecting {
		t.Fatalf("expected selection drag to remain active while wheeling")
	}
	if m.selectionCursor.line <= m.selectionAnchor.line {
		t.Fatalf("expected cursor line to advance while wheeling; anchor=%d cursor=%d", m.selectionAnchor.line, m.selectionCursor.line)
	}
	if m.selectionAnchor.line >= m.stdoutView.YOffset {
		t.Fatalf("expected anchor to be off-screen after wheeling; anchor=%d yOffset=%d", m.selectionAnchor.line, m.stdoutView.YOffset)
	}

	selected := m.selectedText()
	if selected == "" {
		t.Fatalf("expected non-empty selection after drag+wheel")
	}
	expectedLines := m.selectionCursor.line - m.selectionAnchor.line + 1
	if gotLines := strings.Count(selected, "\n") + 1; gotLines != expectedLines {
		t.Fatalf("expected %d selected lines, got %d", expectedLines, gotLines)
	}
}

func TestTailDragSelectionAutoScrollsNearBottom(t *testing.T) {
	m := NewTailModel("1", "", "", 90, 20, TailModeStdout)
	for i := 0; i < 160; i++ {
		m.stdoutLines = append(m.stdoutLines, fmt.Sprintf("line-%03d payload", i))
	}
	m.refreshStdoutContent()

	geom, ok := m.paneGeometry("stdout")
	if !ok {
		t.Fatalf("expected stdout geometry")
	}
	x := geom.contentX + 1
	startY := geom.contentY + 1

	model, _ := m.Update(tea.MouseMsg{
		X:      x,
		Y:      startY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		Type:   tea.MouseLeft,
	})
	m = model.(TailModel)

	dragY := geom.contentY + geom.contentHeight + 2
	for i := 0; i < 8; i++ {
		model, _ = m.Update(tea.MouseMsg{
			X:      x,
			Y:      dragY,
			Action: tea.MouseActionMotion,
			Button: tea.MouseButtonLeft,
			Type:   tea.MouseMotion,
		})
		m = model.(TailModel)
	}

	if m.stdoutView.YOffset <= 0 {
		t.Fatalf("expected drag selection to auto-scroll down, got YOffset=%d", m.stdoutView.YOffset)
	}
	if m.selectionCursor.line <= m.selectionAnchor.line {
		t.Fatalf("expected selection to extend beyond anchor after auto-scroll; anchor=%d cursor=%d", m.selectionAnchor.line, m.selectionCursor.line)
	}

	selected := m.selectedText()
	if selected == "" {
		t.Fatalf("expected non-empty selection after auto-scroll drag")
	}
}

func TestTailSelectionAutoScrollTickContinuesWithoutMouseMotion(t *testing.T) {
	m := NewTailModel("1", "", "", 90, 20, TailModeStdout)
	for i := 0; i < 220; i++ {
		m.stdoutLines = append(m.stdoutLines, fmt.Sprintf("line-%03d payload", i))
	}
	m.refreshStdoutContent()

	geom, ok := m.paneGeometry("stdout")
	if !ok {
		t.Fatalf("expected stdout geometry")
	}
	x := geom.contentX + 1
	startY := geom.contentY + 1

	model, _ := m.Update(tea.MouseMsg{
		X:      x,
		Y:      startY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		Type:   tea.MouseLeft,
	})
	m = model.(TailModel)

	edgeY := geom.contentY + geom.contentHeight + 2
	model, cmd := m.Update(tea.MouseMsg{
		X:      x,
		Y:      edgeY,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		Type:   tea.MouseMotion,
	})
	m = model.(TailModel)

	if cmd == nil {
		t.Fatalf("expected initial drag past edge to schedule auto-scroll")
	}
	if !m.selectionAutoScrollPending {
		t.Fatalf("expected auto-scroll to be pending after edge drag")
	}

	initialOffset := m.stdoutView.YOffset
	initialCursor := m.selectionCursor.line

	for i := 0; i < 3; i++ {
		model, cmd = m.Update(selectionAutoScrollMsg{session: m.session})
		m = model.(TailModel)
	}

	if m.stdoutView.YOffset <= initialOffset {
		t.Fatalf("expected timer-driven auto-scroll to continue without more mouse motion, initial=%d final=%d", initialOffset, m.stdoutView.YOffset)
	}
	if m.selectionCursor.line <= initialCursor {
		t.Fatalf("expected selection cursor to continue advancing, initial=%d final=%d", initialCursor, m.selectionCursor.line)
	}
	if cmd == nil {
		t.Fatalf("expected auto-scroll to keep scheduling while cursor stays beyond the edge")
	}
}

func TestTailIgnoresStaleSessionMessages(t *testing.T) {
	stale := NewTailModel("1", "", "", 80, 12, TailModeStdout)
	current := NewTailModel("1", "", "", 80, 12, TailModeStdout)
	if stale.session == current.session {
		t.Fatalf("expected distinct tail sessions, got %d", current.session)
	}

	current.stdoutLines = append(current.stdoutLines, "current line")
	current.refreshStdoutContent()

	model, _ := current.Update(tailStartMsg{
		session:      stale.session,
		pane:         "stdout",
		initialLines: []string{"stale initial"},
		startErr:     fmt.Errorf("stale"),
	})
	current = model.(TailModel)
	if got := strings.Join(current.stdoutLines, "\n"); strings.Contains(got, "stale initial") {
		t.Fatalf("stale start message should be ignored, got %q", got)
	}

	model, _ = current.Update(logLineMsg{
		session:  stale.session,
		pane:     "stdout",
		err:      io.EOF,
		terminal: true,
	})
	current = model.(TailModel)
	if got := strings.Join(current.stdoutLines, "\n"); strings.Contains(got, "EOF (tail exited)") {
		t.Fatalf("stale EOF should be ignored, got %q", got)
	}
}

func TestTailShowBothStartsMissingPaneFromStdoutMode(t *testing.T) {
	m := NewTailModel("1", "", "", 80, 12, TailModeStdout)

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	updated := model.(TailModel)

	if updated.mode != TailModeBoth {
		t.Fatalf("expected mode to switch to both, got %v", updated.mode)
	}
	if !updated.stdoutStarted {
		t.Fatalf("expected stdout pane to remain started")
	}
	if !updated.stderrStarted {
		t.Fatalf("expected stderr pane to be marked started after switching to both")
	}

	msgs := runTeaCmd(cmd)
	if len(msgs) != 1 {
		t.Fatalf("expected one start message, got %d", len(msgs))
	}

	start, ok := msgs[0].(tailStartMsg)
	if !ok {
		t.Fatalf("expected tailStartMsg, got %T", msgs[0])
	}
	if start.pane != "stderr" {
		t.Fatalf("expected missing stderr pane to start, got %q", start.pane)
	}
}

func TestTailShowStdoutStartsMissingPaneFromStderrMode(t *testing.T) {
	m := NewTailModel("1", "", "", 80, 12, TailModeStderr)

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated := model.(TailModel)

	if updated.mode != TailModeStdout {
		t.Fatalf("expected mode to switch to stdout, got %v", updated.mode)
	}
	if !updated.stdoutStarted {
		t.Fatalf("expected stdout pane to be marked started after switching to stdout")
	}
	if !updated.stderrStarted {
		t.Fatalf("expected stderr pane to remain started")
	}

	msgs := runTeaCmd(cmd)
	if len(msgs) != 1 {
		t.Fatalf("expected one start message, got %d", len(msgs))
	}

	start, ok := msgs[0].(tailStartMsg)
	if !ok {
		t.Fatalf("expected tailStartMsg, got %T", msgs[0])
	}
	if start.pane != "stdout" {
		t.Fatalf("expected missing stdout pane to start, got %q", start.pane)
	}
}
