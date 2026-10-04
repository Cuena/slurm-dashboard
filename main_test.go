package main

import (
	"errors"
	"github.com/charmbracelet/bubbles/table"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	m.toggleDetailsMode()
	if !m.showAllDetails {
		t.Fatalf("expected all-fields mode")
	}
	if got := detailRowsToMap(m.detailsTable.Rows())["GroupId"]; got != "bsc(50000)" {
		t.Fatalf("all-fields mode omitted the previously hidden group: %q", got)
	}
}

type tableExpectation struct {
	key   string
	value string
}

func TestDetailsOverlayLoadsThroughDebounce(t *testing.T) {
	installCommandFixture(t, "scontrol", "JobId=101 Reason=Resources WorkDir=/scratch/project")
	m := NewModel()
	m.detailsDebounce = time.Millisecond
	m.applyWindowSize(50, 16)
	model, _ := m.Update(jobsMsg{requestID: m.jobsRequestID, mode: modeLive, jobs: []Job{{JobID: "101", Status: "PD"}}})
	m = model.(Model)
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = model.(Model)
	if !m.inDetailsOverlay {
		t.Fatal("expected compact-window inspection overlay")
	}
	for _, debounce := range runTeaCmd(cmd) {
		model, fetch := m.Update(debounce)
		m = model.(Model)
		for _, result := range runTeaCmd(fetch) {
			model, _ = m.Update(result)
			m = model.(Model)
		}
	}
	if !strings.Contains(m.View(), "Resources") {
		t.Fatalf("details did not load in overlay: %s", m.View())
	}
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	if m.inDetailsOverlay || !m.table.Focused() {
		t.Fatal("closing inspection must return keyboard navigation to jobs")
	}
}

func TestCancelDialogKeepsPollingAfterDismissal(t *testing.T) {
	installCommandFixture(t, "squeue", "101|train|user|R|gpu|00:01|1|node\n")
	m := NewModel()
	m.liveRefresh = time.Millisecond
	m.confirmingCancel = true
	m.paused = true
	model, cmd := m.Update(tickMsg(time.Now()))
	m = model.(Model)
	ticks := runTeaCmd(cmd)
	if len(ticks) != 1 {
		t.Fatalf("paused poll loop lost its next tick in confirmation: %v", ticks)
	}
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	m.paused = false
	_, cmd = m.Update(ticks[0])
	for _, result := range runTeaCmd(cmd) {
		if jobs, ok := result.(jobsMsg); ok && len(jobs.jobs) == 1 && jobs.jobs[0].JobID == "101" {
			return
		}
	}
	t.Fatal("polling did not fetch jobs after dismissing confirmation")
}

func TestOverlaysReceiveBackgroundResults(t *testing.T) {
	for _, overlay := range []string{"confirmation", "value"} {
		t.Run(overlay, func(t *testing.T) {
			m := NewModel()
			m.jobs = []Job{{JobID: "101", Status: "R"}}
			m.selectedID = "101"
			m.updateTable()
			m.confirmingCancel = overlay == "confirmation"
			m.inValueOverlay = overlay == "value"
			model, _ := m.Update(jobsMsg{requestID: m.jobsRequestID, mode: modeLive, jobs: []Job{{JobID: "101", Status: "R"}}})
			m = model.(Model)
			if job := m.getSelectedJob(); job == nil || job.JobID != "101" {
				t.Fatal("background jobs result was discarded by overlay")
			}
			model, _ = m.Update(errMsg(errors.New("cancel failed")))
			if got := model.(Model).err; got == nil || got.Error() != "cancel failed" {
				t.Fatalf("background action error was discarded: %v", got)
			}
		})
	}
}

func TestRefreshPreservesIdentityAndClearsRemovedDetails(t *testing.T) {
	m := NewModel()
	m.jobs = []Job{{JobID: "101", Status: "R"}, {JobID: "102", Status: "R"}}
	m.selectedID = "102"
	m.updateTable()
	m.rawDetails = "JobId=102 WorkDir=/scratch/old"
	m.updateDetailsTable(m.rawDetails)
	model, _ := m.Update(jobsMsg{requestID: m.jobsRequestID, mode: modeLive, jobs: []Job{m.jobs[1], m.jobs[0]}})
	m = model.(Model)
	if m.selectedJobID() != "102" {
		t.Fatalf("refresh moved selection to %q", m.selectedJobID())
	}
	stale := detailsMsg{requestID: m.detailsRequestID, jobID: "102", text: m.rawDetails}
	model, _ = m.Update(jobsMsg{requestID: m.jobsRequestID, mode: modeLive})
	m = model.(Model)
	model, _ = m.Update(stale)
	m = model.(Model)
	if m.selectedID != "" || m.rawDetails != "" || len(m.detailsTable.Rows()) != 0 {
		t.Fatal("removed job or its late result retained stale details")
	}
}

func TestFilteringInvalidatesPreviousDetailsAndLogRequest(t *testing.T) {
	m := NewModel()
	m.jobs = []Job{{JobID: "101", Name: "train", Status: "R"}, {JobID: "102", Name: "eval", Status: "R"}}
	m.selectedID = "101"
	m.updateTable()
	m.rawDetails = "JobId=101 WorkDir=/scratch/old"
	m.updateDetailsTable(m.rawDetails)
	_ = m.queueTailPathsCmd("101", TailModeStdout)
	stale := tailPathsMsg{requestID: m.tailPathsRequestID, jobID: "101", mode: TailModeStdout}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = model.(Model)
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("eval")})
	m = model.(Model)
	if m.selectedID != "102" || m.rawDetails != "" {
		t.Fatal("filter kept details from the previous selected job")
	}
	model, _ = m.Update(stale)
	if model.(Model).inTailView {
		t.Fatal("obsolete log resolution overrode the new selection")
	}
}

func TestTailResizeSurvivesReturnToJobs(t *testing.T) {
	m := NewModel()
	m.applyWindowSize(120, 40)
	m.tailModel = NewTailModel("101", "", "", 120, 40, TailModeStdout)
	m.inTailView = true
	model, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m = model.(Model)
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	width, height := measureView(m.View())
	if m.inTailView || width > 70 || height > 20 {
		t.Fatalf("returned main view does not fit resized terminal: %dx%d", width, height)
	}
}

func TestLateTailStartupAfterExitClosesPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	m := NewModel()
	m.tailModel = NewTailModel("101", "", "", 80, 24, TailModeStdout)
	m.inTailView = true
	stale := tailStartMsg{session: m.tailModel.session, pane: "stdout", pipe: r}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	model, cleanup := m.Update(stale)
	runTeaCmd(cleanup)
	if model.(Model).inTailView {
		t.Fatal("late startup reopened the log view")
	}
	if _, err := r.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("late startup pipe remains open: %v", err)
	}
}

func TestPollingDoesNotStarvePendingDetails(t *testing.T) {
	installCommandFixture(t, "scontrol", "JobId=101 Reason=Resources")
	m := NewModel()
	m.applyWindowSize(120, 40)
	m.jobs = []Job{{JobID: "101", Status: "PD"}}
	m.selectedID = "101"
	m.updateTable()
	m.detailsDebounce = time.Millisecond
	debounce := m.queueDetailsFetchCmd("101")
	refresh := jobsMsg{requestID: m.jobsRequestID, mode: modeLive, jobs: m.jobs}
	model, _ := m.Update(refresh)
	m = model.(Model)
	for _, msg := range runTeaCmd(debounce) {
		model, fetch := m.Update(msg)
		m = model.(Model)
		model, _ = m.Update(refresh)
		m = model.(Model)
		for _, result := range runTeaCmd(fetch) {
			model, _ = m.Update(result)
			m = model.(Model)
		}
	}
	if !strings.Contains(m.View(), "Resources") {
		t.Fatal("polling invalidated a pending debounce or details command")
	}
}
