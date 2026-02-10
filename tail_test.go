package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
