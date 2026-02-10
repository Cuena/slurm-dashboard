package main

import "testing"

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
