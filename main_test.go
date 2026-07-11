package main

import (
	"errors"
	"testing"
	"time"
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

func TestModelIgnoresStaleTailPathsResponse(t *testing.T) {
	m := NewModel()
	m.tailPathsRequestID = 2

	model, _ := m.Update(tailPathsMsg{
		requestID: 1,
		jobID:     "old",
		stdout:    "/tmp/old.out",
		mode:      TailModeStdout,
	})
	updated := model.(Model)
	if updated.inTailView {
		t.Fatalf("expected stale log-path response to be ignored")
	}
}

func TestJobsErrorClearsLoadingState(t *testing.T) {
	m := NewModel()
	m.appMode = modeHistory
	m.jobsRequestID = 7
	m.loadingJobs = true

	model, _ := m.Update(jobsErrMsg{requestID: 7, mode: modeHistory, err: errors.New("sacct unavailable")})
	updated := model.(Model)
	if updated.loadingJobs {
		t.Fatalf("expected matching jobs error to clear loading state")
	}
	if updated.err == nil {
		t.Fatalf("expected matching jobs error to be visible")
	}
}

func TestDurationFromEnv(t *testing.T) {
	t.Setenv("TEST_REFRESH", "45s")
	if got := durationFromEnv("TEST_REFRESH", time.Minute, time.Second); got != 45*time.Second {
		t.Fatalf("expected 45s, got %s", got)
	}
	t.Setenv("TEST_REFRESH", "30")
	if got := durationFromEnv("TEST_REFRESH", time.Minute, time.Second); got != 30*time.Second {
		t.Fatalf("expected integer seconds, got %s", got)
	}
}

type tableExpectation struct {
	key   string
	value string
}
