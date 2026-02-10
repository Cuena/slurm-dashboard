package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestViewFitsInWindow(t *testing.T) {
	model := NewModel()
	model.jobs = sampleJobs()
	model.updateTable()
	model.filtered = model.jobs
	model.selectedID = model.jobs[0].JobID
	model.lastRefresh = time.Now()
	model.rawDetails = "Key=Value\nFoo=Bar Baz"
	model.updateDetailsTable(model.rawDetails)

	// Ensure we have a detail row selected for inspector calculations.
	if len(model.detailsTable.Rows()) > 0 {
		model.detailsTable.SetCursor(0)
	}

	sizes := []struct {
		w int
		h int
	}{
		{120, 40},
		{100, 30},
		{80, 24},
		{70, 20},
		{60, 18},
		{55, 18},
		{50, 16},
	}

	for _, size := range sizes {
		model.applyWindowSize(size.w, size.h)
		view := model.View()
		vw, vh := measureView(view)
		if vw > size.w {
			t.Fatalf("view width %d exceeds window width %d (height %d)", vw, size.w, size.h)
		}
		if vh > size.h {
			header := model.renderHeaderArea()
			tablePanel := model.renderTablePanel()
			detailsPanel := model.renderDetailsPanel()
			gap := lipgloss.NewStyle().Width(panelGap).Render(" ")
			mainView := detailsPanel
			if model.stackPanels {
				mainView = lipgloss.JoinVertical(lipgloss.Left, tablePanel, detailsPanel)
			} else {
				mainView = lipgloss.JoinHorizontal(lipgloss.Top, tablePanel, gap, detailsPanel)
			}
			helpView := model.help.View(keys)
			headerH := lipgloss.Height(header)
			mainH := lipgloss.Height(mainView)
			helpH := lipgloss.Height(helpView)
			t.Fatalf("view height %d exceeds window height %d (width %d) [header=%d main=%d help=%d]", vh, size.h, size.w, headerH, mainH, helpH)
		}
	}
}

func sampleJobs() []Job {
	return []Job{
		{JobID: "101", Name: "train", User: "alice", Status: "RUNNING", Partition: "gpu", Time: "00:10:00", Nodes: "1", NodeList: "node001"},
		{JobID: "102", Name: "eval", User: "bob", Status: "PENDING", Partition: "cpu", Time: "00:01:00", Nodes: "2", NodeList: "node[002-003]"},
	}
}

func measureView(view string) (width int, height int) {
	clean := strings.ReplaceAll(view, "\r\n", "\n")
	lines := strings.Split(clean, "\n")
	height = len(lines)
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > width {
			width = w
		}
	}
	return
}
