package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSqueueOutput(t *testing.T) {
	output := `34989208|vllm_qwen2_5_72b_instruct_default_gpu4_tp4|bsc070916|R|acc|2:22|1|as02r3b15
34989209|another_job|bsc070916|PD|acc|0:00|0|`
	jobs := parseSqueue(output)

	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	if jobs[0].JobID != "34989208" {
		t.Errorf("expected job ID 34989208, got %s", jobs[0].JobID)
	}
	if jobs[0].Status != "R" {
		t.Errorf("expected status R, got %s", jobs[0].Status)
	}
	if jobs[0].NodeList != "as02r3b15" {
		t.Errorf("expected nodelist as02r3b15, got %s", jobs[0].NodeList)
	}
}

func TestParseSacctOutput(t *testing.T) {
	// Real output from the audit - includes step entries
	output := `34949712|vllm_glm4_6_tp16_ray_manual_4x4|bsc070916|CANCELLED by 4840|acc|00:40:07|4|as04r3b19,as04r5b[26-28]
34952064|vllm_glm4_6_tp16_ray_manual_4x4|bsc070916|CANCELLED by 4840|acc|00:11:25|4|as02r3b[01-04]
34989208|vllm_qwen2_5_72b_instruct_default_gpu4_tp4|bsc070916|RUNNING|acc|00:02:22|1|as02r3b15`
	jobs := parseSacct(output)

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	// parseSacct reverses order (newest first)
	if jobs[0].JobID != "34989208" {
		t.Errorf("expected job ID 34989208 first (most recent), got %s", jobs[0].JobID)
	}
	if jobs[0].Status != "RUNNING" {
		t.Errorf("expected status RUNNING, got %s", jobs[0].Status)
	}
}

func TestParseSacctSkipsStepEntries(t *testing.T) {
	// Output that includes .batch and .extern step entries
	output := `34989208|vllm_qwen2_5_72b|bsc070916|RUNNING|acc|00:02:22|1|as02r3b15
34989208.batch|batch||RUNNING||00:02:22|1|as02r3b15
34989208.extern|extern||RUNNING||00:02:22|1|as02r3b15`
	jobs := parseSacct(output)

	if len(jobs) != 1 {
		t.Fatalf("expected 1 job (step entries should be skipped), got %d", len(jobs))
	}
	if jobs[0].JobID != "34989208" {
		t.Errorf("expected job ID 34989208, got %s", jobs[0].JobID)
	}
}

func TestStateCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"RUNNING", "R"},
		{"PENDING", "PD"},
		{"COMPLETED", "CD"},
		{"CANCELLED by 4840", "CA"},
		{"CANCELLED", "CA"},
		{"R", "R"},
		{"PD", "PD"},
		{"TIMEOUT", "TO"},
		{"FAILED", "F"},
		{"OUT_OF_MEMORY", "OOM"},
		{"", ""},
	}

	for _, tc := range tests {
		got := StateCode(tc.input)
		if got != tc.expected {
			t.Errorf("StateCode(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestResolveLogPathExpandsRelative(t *testing.T) {
	got := resolveLogPath("slurm_output/%x_%j.out", "/work", "35121055", "susy_nc_cpu")
	want := "/work/slurm_output/susy_nc_cpu_35121055.out"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestParseSubmitLineScriptPath(t *testing.T) {
	submitLine := "sbatch -A acc --chdir=/work /tmp/job.sbatch"
	path := parseSubmitLineScriptPath(submitLine)
	if path != "/tmp/job.sbatch" {
		t.Fatalf("expected script path /tmp/job.sbatch, got %q", path)
	}
}

func TestReadSbatchDirectives(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "job.sbatch")
	contents := "#!/bin/bash\n#SBATCH --chdir=/work\n#SBATCH --output=slurm_output/%x_%j.out\n#SBATCH --error=slurm_output/%x_%j.err\n"
	if err := os.WriteFile(scriptPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	directives, err := readSbatchDirectives(scriptPath)
	if err != nil {
		t.Fatalf("read directives: %v", err)
	}
	if directives.stdout != "slurm_output/%x_%j.out" {
		t.Fatalf("expected stdout directive, got %q", directives.stdout)
	}
	if directives.stderr != "slurm_output/%x_%j.err" {
		t.Fatalf("expected stderr directive, got %q", directives.stderr)
	}
	if directives.chdir != "/work" {
		t.Fatalf("expected chdir directive, got %q", directives.chdir)
	}
}

func TestResolveArchiveConventionPathsJobIDFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLURM_DASHBOARD_LOG_ARCHIVE_DIR", dir)

	jobID := "12345"
	stdoutPath := filepath.Join(dir, jobID+".out")
	stderrPath := filepath.Join(dir, jobID+".err")

	if err := os.WriteFile(stdoutPath, []byte("stdout"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("stderr"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	gotOut, gotErr, ok := resolveArchiveConventionPaths(jobID)
	if !ok {
		t.Fatalf("expected archive convention to resolve paths")
	}
	if gotOut != stdoutPath {
		t.Fatalf("expected stdout %q, got %q", stdoutPath, gotOut)
	}
	if gotErr != stderrPath {
		t.Fatalf("expected stderr %q, got %q", stderrPath, gotErr)
	}
}

func TestResolveArchiveConventionPathsMergedOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLURM_DASHBOARD_LOG_ARCHIVE_DIR", dir)

	jobID := "67890"
	mergedPath := filepath.Join(dir, "slurm-"+jobID+".out")
	if err := os.WriteFile(mergedPath, []byte("merged"), 0o600); err != nil {
		t.Fatalf("write merged log: %v", err)
	}

	gotOut, gotErr, ok := resolveArchiveConventionPaths(jobID)
	if !ok {
		t.Fatalf("expected archive convention to resolve merged output")
	}
	if gotOut != mergedPath {
		t.Fatalf("expected stdout %q, got %q", mergedPath, gotOut)
	}
	if gotErr != mergedPath {
		t.Fatalf("expected stderr to fall back to stdout %q, got %q", mergedPath, gotErr)
	}
}
