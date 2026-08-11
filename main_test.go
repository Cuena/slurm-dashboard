package main

import (
	"errors"
	"github.com/charmbracelet/bubbles/table"
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

func TestCuratedDetailsPrioritizePendingInsight(t *testing.T) {
	input := "JobId=456 JobName=train JobState=PENDING Reason=Resources StartTime=2026-03-10T15:00:00 EligibleTime=2026-03-10T14:30:00 SubmitTime=2026-03-10T14:00:00 Priority=12345 Partition=gpu"

	rows := curatedDetailRows(parseDetailsToRows(input))
	if len(rows) < 5 {
		t.Fatalf("expected pending summary rows to be prepended, got %d rows", len(rows))
	}

	want := []tableExpectation{
		{key: "Reason", value: "Resources"},
		{key: "Priority", value: "12345"},
		{key: "Submitted", value: "2026-03-10T14:00:00"},
		{key: "Eligible", value: "2026-03-10T14:30:00"},
		{key: "Started", value: "2026-03-10T15:00:00"},
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

func TestCuratedDetailsUseOperationalSlurmFields(t *testing.T) {
	input := "JobId=43169753 JobName=sam3 JobState=PENDING Reason=Priority Priority=107874 QOS=acc_debug Account=bsc70 SubmitTime=2026-07-12T00:26:27 EligibleTime=2026-07-12T00:26:27 RunTime=00:00:00 TimeLimit=02:00:00 NumTasks=4 NumCPUs=16 ReqTRES=cpu=16,mem=64G WorkDir=/gpfs/scratch/run StdOut=/gpfs/scratch/run/job.out Command=/gpfs/scratch/run/job.sh GroupId=bsc(50000)"
	rows := curatedDetailRows(parseDetailsToRows(input))
	fields := detailRowsToMap(rows)

	want := map[string]string{
		"Reason":         "Priority",
		"Priority":       "107874",
		"QOS":            "acc_debug",
		"Runtime":        "00:00:00",
		"Time limit":     "02:00:00",
		"Tasks":          "4",
		"CPUs":           "16",
		"Requested TRES": "cpu=16,mem=64G",
		"Work directory": "/gpfs/scratch/run",
		"Stdout":         "/gpfs/scratch/run/job.out",
		"Command":        "/gpfs/scratch/run/job.sh",
	}
	for key, expected := range want {
		if got := fields[key]; got != expected {
			t.Fatalf("curated field %q = %q, want %q", key, got, expected)
		}
	}
	if _, ok := fields["GroupId"]; ok {
		t.Fatalf("expected low-value raw field to be hidden in curated mode")
	}
}

func TestToggleDetailsModeRestoresAllRawFields(t *testing.T) {
	m := NewModel()
	m.rawDetails = "JobId=1 JobState=PENDING Reason=Priority GroupId=bsc(50000) Priority=42"
	m.updateDetailsTable(m.rawDetails)
	curatedCount := len(m.detailsTable.Rows())
	m.toggleDetailsMode()
	if !m.showAllDetails {
		t.Fatalf("expected all-fields mode")
	}
	if got := len(m.detailsTable.Rows()); got <= curatedCount {
		t.Fatalf("expected all fields to contain more rows: curated=%d all=%d", curatedCount, got)
	}
}

func TestTransparentSurfacesAreDefault(t *testing.T) {
	if got := parseSurfaceMode(""); got != SurfaceTransparent {
		t.Fatalf("default surface mode = %q, want transparent", got)
	}
}

type tableExpectation struct {
	key   string
	value string
}
