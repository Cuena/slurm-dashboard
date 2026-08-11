package main

import (
	"errors"
	"testing"

	"github.com/charmbracelet/bubbles/table"
)

func TestParseDetailsToRowsPreservesValuesWithSpaces(t *testing.T) {
	input := "JobId=123 JobName=train JobState=RUNNING Reason=None Command=/bin/bash -lc 'python train.py --arg=1' WorkDir=/scratch/my project"

	rows := parseDetailsToRows(input)
	fields := detailRowsToMap(rows)

	if got := fields["Command"]; got != "/bin/bash -lc 'python train.py --arg=1'" {
		t.Fatalf("expected full command value, got %q", got)
	}
	if got := fields["WorkDir"]; got != "/scratch/my project" {
		t.Fatalf("expected workdir with spaces, got %q", got)
	}
}

func TestParseDetailsToRowsPrependsPendingInsight(t *testing.T) {
	input := "JobId=456 JobName=train JobState=PENDING Reason=Resources StartTime=2026-03-10T15:00:00 EligibleTime=2026-03-10T14:30:00 SubmitTime=2026-03-10T14:00:00 Priority=12345 Partition=gpu"

	rows := parseDetailsToRows(input)
	if len(rows) < 5 {
		t.Fatalf("expected pending summary rows to be prepended, got %d rows", len(rows))
	}

	want := []tableExpectation{
		{key: "PendingReason", value: "Resources"},
		{key: "ExpectedStart", value: "2026-03-10T15:00:00"},
		{key: "EligibleTime", value: "2026-03-10T14:30:00"},
		{key: "SubmitTime", value: "2026-03-10T14:00:00"},
		{key: "Priority", value: "12345"},
	}

	for i, expected := range want {
		if rows[i][0] != expected.key || rows[i][1] != expected.value {
			t.Fatalf("row %d = %v, want [%s %s]", i, rows[i], expected.key, expected.value)
		}
	}
}

func TestModelIgnoresStaleJobsResponse(t *testing.T) {
	m := NewModel()
	m.appMode = modeHistory
	m.jobsRequestID = 3

	model, _ := m.Update(jobsMsg{
		requestID: 2,
		mode:      modeLive,
		jobs:      []Job{{JobID: "1", Status: "R"}},
	})
	updated := model.(Model)

	if len(updated.jobs) != 0 {
		t.Fatalf("expected stale jobs response to be ignored, got %+v", updated.jobs)
	}
}

func TestModelIgnoresStaleJobsError(t *testing.T) {
	m := NewModel()
	m.appMode = modeLive
	m.jobsRequestID = 3

	model, _ := m.Update(jobsErrMsg{
		requestID: 2,
		mode:      modeLive,
		err:       errors.New("command timed out"),
	})
	updated := model.(Model)

	if updated.err != nil {
		t.Fatalf("expected stale jobs error to be ignored, got %v", updated.err)
	}
}

func TestModelClearsErrorOnJobsSuccess(t *testing.T) {
	m := NewModel()
	m.appMode = modeLive
	m.jobsRequestID = 3
	m.err = errors.New("command timed out")

	model, _ := m.Update(jobsMsg{
		requestID: 3,
		mode:      modeLive,
		jobs:      []Job{{JobID: "1", Status: "R"}},
	})
	updated := model.(Model)

	if updated.err != nil {
		t.Fatalf("expected jobs success to clear error, got %v", updated.err)
	}
}

func TestModelIgnoresStaleDetailsResponse(t *testing.T) {
	m := NewModel()
	m.selectedID = "current"
	m.detailsRequestID = 4

	model, _ := m.Update(detailsMsg{
		requestID: 3,
		jobID:     "old",
		history:   false,
		text:      "JobId=old",
	})
	updated := model.(Model)

	if updated.rawDetails != "" {
		t.Fatalf("expected stale details response to be ignored, got %q", updated.rawDetails)
	}
}

func TestJobTableRowMapsValuesByColumnTitle(t *testing.T) {
	row := newJobTableRow(Job{
		JobID:  "123",
		Name:   "train",
		Status: "RUNNING",
	}).tableRow([]table.Column{
		{Title: jobColumnStatus},
		{Title: jobColumnID},
		{Title: jobColumnName},
	})

	if got, want := row[0], "R"; got != want {
		t.Fatalf("status cell = %q, want %q", got, want)
	}
	if got, want := row[1], "123"; got != want {
		t.Fatalf("id cell = %q, want %q", got, want)
	}
	if got, want := row[2], "train"; got != want {
		t.Fatalf("name cell = %q, want %q", got, want)
	}
}

func TestSelectedJobUsesFilteredCursorNotTableRowPosition(t *testing.T) {
	m := NewModel()
	m.jobs = []Job{
		{JobID: "101", Name: "first", Status: "RUNNING"},
		{JobID: "102", Name: "second", Status: "PENDING"},
	}
	m.table.SetColumns([]table.Column{
		{Title: jobColumnName, Width: 12},
		{Title: jobColumnID, Width: 8},
	})
	m.updateTable()
	m.table.SetCursor(1)

	if got, want := m.selectedJobID(), "102"; got != want {
		t.Fatalf("selected job ID = %q, want %q", got, want)
	}
	if job := m.getSelectedJob(); job == nil || job.JobID != "102" {
		t.Fatalf("selected job = %+v, want job 102", job)
	}
}

type tableExpectation struct {
	key   string
	value string
}
