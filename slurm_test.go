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
	output := `34949712|vllm_glm4_6_tp16_ray_manual_4x4|bsc070916|CANCELLED by 4840|acc|00:40:07|4|as04r3b19,as04r5b[26-28]
34952064|vllm_glm4_6_tp16_ray_manual_4x4|bsc070916|CANCELLED by 4840|acc|00:11:25|4|as02r3b[01-04]
34989208|vllm_qwen2_5_72b_instruct_default_gpu4_tp4|bsc070916|RUNNING|acc|00:02:22|1|as02r3b15`
	jobs := parseSacct(output)

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	if jobs[0].JobID != "34989208" {
		t.Errorf("expected job ID 34989208 first (most recent), got %s", jobs[0].JobID)
	}
	if jobs[0].Status != "RUNNING" {
		t.Errorf("expected status RUNNING, got %s", jobs[0].Status)
	}
}

func TestParseSacctSkipsStepEntries(t *testing.T) {
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

func TestParseSubmitLineDirectivesHandlesQuotedValues(t *testing.T) {
	submitLine := `sbatch --chdir "/scratch/my project" --output "logs/%x %j.out" --error=errs/%j.err train.sbatch`

	got := parseSubmitLineDirectives(submitLine)

	if got.chdir != "/scratch/my project" {
		t.Fatalf("expected quoted chdir to be preserved, got %q", got.chdir)
	}
	if got.stdout != "logs/%x %j.out" {
		t.Fatalf("expected quoted stdout path to be preserved, got %q", got.stdout)
	}
	if got.stderr != "errs/%j.err" {
		t.Fatalf("expected stderr path, got %q", got.stderr)
	}
}

func TestParseSubmitLineScriptPathHandlesQuotedFlags(t *testing.T) {
	submitLine := `sbatch --wrap "python train.py --lr 1e-3" --chdir "/scratch/my project" scripts/run experiment.sbatch`

	got := parseSubmitLineScriptPath(submitLine)

	if got != "scripts/run" {
		t.Fatalf("expected first positional script path, got %q", got)
	}
}

func TestResolveSubmitLineScriptPathUsesWorkDirForRelativePaths(t *testing.T) {
	got := resolveSubmitLineScriptPath("sbatch scripts/train.sbatch", "/scratch/run-42")
	want := "/scratch/run-42/scripts/train.sbatch"
	if got != want {
		t.Fatalf("expected resolved script path %q, got %q", want, got)
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

func TestParseSbatchDirectivesHandlesQuotedDirectiveValues(t *testing.T) {
	contents := `
#SBATCH --output "logs/%x %j.out"
#SBATCH --error errs/%j.err
#SBATCH --chdir "/scratch/my project"
`

	got := parseSbatchDirectives(contents)

	if got.stdout != "logs/%x %j.out" {
		t.Fatalf("expected stdout directive to preserve spaces, got %q", got.stdout)
	}
	if got.stderr != "errs/%j.err" {
		t.Fatalf("expected stderr directive, got %q", got.stderr)
	}
	if got.chdir != "/scratch/my project" {
		t.Fatalf("expected chdir directive to preserve spaces, got %q", got.chdir)
	}
}

func TestParseSacctLogInfoPreservesPipesInSubmitLine(t *testing.T) {
	output := `/work|train|/logs/train-123.out|/logs/train-123.err|sbatch --wrap "python -c 'print(\"a|b\")'"`

	got, ok := parseSacctLogInfo(output)
	if !ok {
		t.Fatalf("expected sacct log info to parse")
	}
	if got.workDir != "/work" {
		t.Fatalf("expected workdir /work, got %q", got.workDir)
	}
	if got.jobName != "train" {
		t.Fatalf("expected job name train, got %q", got.jobName)
	}
	if got.stdout != "/logs/train-123.out" {
		t.Fatalf("expected stdout path, got %q", got.stdout)
	}
	if got.stderr != "/logs/train-123.err" {
		t.Fatalf("expected stderr path, got %q", got.stderr)
	}
	if got.submitLine != `sbatch --wrap "python -c 'print(\"a|b\")'"` {
		t.Fatalf("expected submit line to preserve pipes, got %q", got.submitLine)
	}
}

func TestResolveSacctLogPathsPrefersDirectStdPaths(t *testing.T) {
	info := sacctLogInfo{
		workDir:    "/work",
		jobName:    "train",
		stdout:     "/logs/train-123.out",
		stderr:     "/logs/train-123.err",
		submitLine: "sbatch train.sbatch",
	}

	gotOut, gotErr, ok := resolveSacctLogPaths(info, "123")
	if !ok {
		t.Fatalf("expected direct sacct paths to resolve")
	}
	if gotOut != "/logs/train-123.out" {
		t.Fatalf("expected stdout path, got %q", gotOut)
	}
	if gotErr != "/logs/train-123.err" {
		t.Fatalf("expected stderr path, got %q", gotErr)
	}
}

func TestResolveSacctLogPathsFallsBackToRelativeScriptDirectives(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "scripts", "train.sbatch")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}
	contents := "#!/bin/bash\n#SBATCH --output=logs/%x_%j.out\n#SBATCH --error=logs/%x_%j.err\n"
	if err := os.WriteFile(scriptPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	info := sacctLogInfo{
		workDir:    dir,
		jobName:    "train",
		submitLine: "sbatch scripts/train.sbatch",
	}

	gotOut, gotErr, ok := resolveSacctLogPaths(info, "123")
	if !ok {
		t.Fatalf("expected script directives to resolve log paths")
	}
	if gotOut != filepath.Join(dir, "logs", "train_123.out") {
		t.Fatalf("expected stdout path from script directives, got %q", gotOut)
	}
	if gotErr != filepath.Join(dir, "logs", "train_123.err") {
		t.Fatalf("expected stderr path from script directives, got %q", gotErr)
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
