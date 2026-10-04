package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHistoryModeFiltersPendingAndRunningJobs(t *testing.T) {
	m := NewModel()
	m.jobs = []Job{
		{JobID: "1", Status: "PD"},
		{JobID: "2", Status: "RUNNING"},
		{JobID: "3", Status: "COMPLETED"},
	}
	m.appMode = modeHistory

	m.updateTable()

	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 job after filtering, got %d", len(m.filtered))
	}
	if m.filtered[0].JobID != "3" {
		t.Fatalf("expected job 3 to remain, got %s", m.filtered[0].JobID)
	}
}

func TestHistoryResetsLiveFilterAndCyclesTerminalStates(t *testing.T) {
	m := NewModel()
	m.sFilter = filterRunning
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = model.(Model)
	jobs := []Job{
		{JobID: "1", Status: "FAILED"},
		{JobID: "2", Status: "OUT_OF_MEMORY"},
		{JobID: "3", Status: "COMPLETED"},
		{JobID: "4", Status: "CANCELLED by 1234"},
		{JobID: "5", Status: "RUNNING"},
	}
	model, _ = m.Update(jobsMsg{requestID: m.jobsRequestID, mode: modeHistory, jobs: jobs})
	m = model.(Model)
	for _, want := range []string{"1,2,3,4", "1,2", "3", "4", "1,2,3,4"} {
		ids := make([]string, 0, len(m.filtered))
		for _, job := range m.filtered {
			ids = append(ids, job.JobID)
		}
		if got := strings.Join(ids, ","); got != want {
			t.Fatalf("history filter %s displayed %q, want %q", m.sFilter, got, want)
		}
		model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
		m = model.(Model)
	}
}
