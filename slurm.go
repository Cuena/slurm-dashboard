package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Job represents a Slurm job
type Job struct {
	JobID     string
	Name      string
	User      string
	Status    string
	Partition string
	Time      string
	Nodes     string
	NodeList  string
}

// State returns the short state code (R, PD, etc.)
func (j Job) State() string {
	return StateCode(j.Status)
}

// IsRunning checks if the job is in a running state
func (j Job) IsRunning() bool {
	s := j.State()
	return s == "R" || s == "CG"
}

// IsPending checks if the job is in a pending state
func (j Job) IsPending() bool {
	s := j.State()
	return s == "PD" || s == "CF" || s == "PR" || s == "RQ" || s == "RS" || s == "S" || s == "ST" || s == "RH" || s == "RF"
}

// IsHistorical reports whether a job is in a terminal state (completed/failed/etc.)
func (j Job) IsHistorical() bool {
	return !j.IsRunning() && !j.IsPending()
}

var statusAliases = map[string]string{
	"RUNNING":       "R",
	"COMPLETING":    "CG",
	"CONFIGURING":   "CF",
	"PENDING":       "PD",
	"PREEMPTED":     "PR",
	"REQUEUED":      "RQ",
	"REQUEUE_HOLD":  "RH",
	"REQUEUE_FED":   "RF",
	"RESIZING":      "RS",
	"SUSPENDED":     "S",
	"STOPPED":       "ST",
	"PP":            "PD", // Handle 'pp' as pending
	"COMPLETED":     "CD",
	"CANCELLED":     "CA",
	"FAILED":        "F",
	"TIMEOUT":       "TO",
	"NODE_FAIL":     "NF",
	"OUT_OF_MEMORY": "OOM",
}

// StateCode converts full status to short code
func StateCode(status string) string {
	text := strings.ToUpper(strings.TrimSpace(status))
	if text == "" {
		return ""
	}
	text = strings.TrimRight(text, "*+")

	// Handle cases like "CANCELLED BY USER" by taking just the first word
	// But we need to check if first word is enough.
	// For "CANCELLED BY USER", first word "CANCELLED" maps to "CA".

	// First check exact match (after trim)
	if alias, ok := statusAliases[text]; ok {
		return alias
	}

	// Try first word match
	parts := strings.Fields(text)
	if len(parts) > 1 {
		if alias, ok := statusAliases[parts[0]]; ok {
			return alias
		}
	}

	return text
}

func CurrentUser() string {
	u, err := user.Current()
	if err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

func RunCommand(args []string, timeout time.Duration) (string, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s: %v, stderr: %s", timeout, err, stderr.String())
	}
	if err != nil {
		return "", fmt.Errorf("command failed: %v, stderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// FetchJobsSqueue fetches jobs using squeue
func FetchJobsSqueue() ([]Job, error) {
	user := CurrentUser()
	format := "%i|%j|%u|%t|%P|%M|%D|%N"

	out, err := RunCommand([]string{"squeue", "-u", user, "-o", format, "--noheader"}, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return parseSqueue(out), nil
}

func parseSqueue(output string) []Job {
	var jobs []Job
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 7 {
			parts = strings.Split(line, "\t")
			if len(parts) < 7 {
				continue
			}
		}

		job := Job{
			JobID:     strings.TrimSpace(parts[0]),
			Name:      strings.TrimSpace(parts[1]),
			User:      strings.TrimSpace(parts[2]),
			Status:    strings.TrimSpace(parts[3]),
			Partition: strings.TrimSpace(parts[4]),
			Time:      strings.TrimSpace(parts[5]),
			Nodes:     strings.TrimSpace(parts[6]),
		}
		if len(parts) > 7 {
			job.NodeList = strings.TrimSpace(parts[7])
		}
		jobs = append(jobs, job)
	}
	return jobs
}

// FetchJobsHistory fetches jobs using sacct (N day history)
func FetchJobsHistory(days int) ([]Job, error) {
	user := CurrentUser()
	startTime := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	args := []string{
		"sacct", "-u", user,
		"--format", "JobID,JobName,User,State,Partition,Elapsed,AllocNodes,NodeList",
		"-X", "-P", "-n",
		"--starttime", startTime,
	}

	out, err := RunCommand(args, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return parseSacct(out), nil
}

func parseSacct(output string) []Job {
	var jobs []Job
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 8 {
			continue
		}

		jobID := parts[0]
		if strings.Contains(jobID, ".") {
			continue
		}

		job := Job{
			JobID:     strings.TrimSpace(parts[0]),
			Name:      strings.TrimSpace(parts[1]),
			User:      strings.TrimSpace(parts[2]),
			Status:    strings.TrimSpace(parts[3]),
			Partition: strings.TrimSpace(parts[4]),
			Time:      strings.TrimSpace(parts[5]),
			Nodes:     strings.TrimSpace(parts[6]),
			NodeList:  strings.TrimSpace(parts[7]),
		}
		jobs = append(jobs, job)
	}
	for i, j := 0, len(jobs)-1; i < j; i, j = i+1, j-1 {
		jobs[i], jobs[j] = jobs[j], jobs[i]
	}
	return jobs
}

// CancelJob cancels a job
func CancelJob(jobID string) error {
	_, err := RunCommand([]string{"scancel", jobID}, 5*time.Second)
	return err
}

// GetJobDetails fetches details for a job
func GetJobDetails(jobID string, history bool) (string, error) {
	if history {
		args := []string{
			"sacct", "-j", jobID,
			"--format", "JobID,JobName,User,State,Partition,Elapsed,AllocNodes,NodeList,Start,End,ExitCode",
			"-P", "-n",
		}
		return RunCommand(args, 15*time.Second)
	}
	return RunCommand([]string{"scontrol", "show", "job", jobID}, 15*time.Second)
}

// ResolveLogPaths finds StdOut and StdErr paths for a job.
// For live/running jobs, it uses scontrol which has the exact paths.
// For finished jobs (or if scontrol fails), it falls back to sacct heuristics.
func ResolveLogPaths(jobID string) (string, string, error) {
	// Try scontrol first (works for jobs still in slurmctld memory)
	out, err := RunCommand([]string{"scontrol", "show", "job", jobID}, 10*time.Second)
	if err == nil {
		stdoutRegex := regexp.MustCompile(`StdOut=(\S+)`)
		stderrRegex := regexp.MustCompile(`StdErr=(\S+)`)

		stdout := ""
		if matches := stdoutRegex.FindStringSubmatch(out); len(matches) > 1 {
			stdout = matches[1]
		}

		stderr := ""
		if matches := stderrRegex.FindStringSubmatch(out); len(matches) > 1 {
			stderr = matches[1]
		}

		// If we found paths, return them
		if stdout != "" || stderr != "" {
			return stdout, stderr, nil
		}
	}

	// Fallback: Use sacct for finished/historical jobs
	// NOTE: sacct doesn't provide StdOut/StdErr directly, so we use heuristics:
	// 1. Parse -o/--output and -e/--error from SubmitLine if present
	// 2. If SubmitLine references a script, parse #SBATCH directives
	// 3. Default to WorkDir/slurm-JOBID.out
	//
	// Using -X to get only the main job entry (skip .batch, .extern steps which have empty WorkDir)
	outSacct, errSacct := RunCommand([]string{"sacct", "-j", jobID, "-o", "WorkDir,SubmitLine,JobName", "-X", "-n", "-P"}, 5*time.Second)
	if errSacct == nil {
		lines := strings.Split(strings.TrimSpace(outSacct), "\n")
		workDir := ""
		submitLine := ""
		jobName := ""

		// Find the first line with a non-empty WorkDir
		// (step entries like .batch/.extern have empty WorkDir)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, "|", 3)
			if len(parts) < 3 {
				continue
			}

			wd := strings.TrimSpace(parts[0])
			if wd == "" {
				// Skip entries with empty WorkDir (step entries)
				continue
			}

			workDir = wd
			submitLine = strings.TrimSpace(parts[1])
			jobName = strings.TrimSpace(parts[2])
			break
		}

		if workDir != "" {
			submitDirectives := parseSubmitLineDirectives(submitLine)
			baseDir := workDir
			if submitDirectives.chdir != "" {
				baseDir = submitDirectives.chdir
			}

			stdoutPath := resolveLogPath(submitDirectives.stdout, baseDir, jobID, jobName)
			stderrPath := resolveLogPath(submitDirectives.stderr, baseDir, jobID, jobName)

			if stdoutPath == "" || stderrPath == "" {
				if scriptPath := parseSubmitLineScriptPath(submitLine); scriptPath != "" {
					if scriptDirectives, err := readSbatchDirectives(scriptPath); err == nil {
						scriptBase := baseDir
						if scriptDirectives.chdir != "" {
							scriptBase = scriptDirectives.chdir
						}
						if stdoutPath == "" {
							stdoutPath = resolveLogPath(scriptDirectives.stdout, scriptBase, jobID, jobName)
						}
						if stderrPath == "" {
							stderrPath = resolveLogPath(scriptDirectives.stderr, scriptBase, jobID, jobName)
						}
					}
				}
			}

			if stdoutPath == "" {
				stdoutPath = resolveLogPath(fmt.Sprintf("slurm-%s.out", jobID), workDir, jobID, jobName)
			}
			if stderrPath == "" {
				stderrPath = stdoutPath
			}

			if stdoutPath != "" || stderrPath != "" {
				return stdoutPath, stderrPath, nil
			}
		}
	}

	// Final fallback for old jobs: a deterministic archive convention that does
	// not depend on slurmctld/sacct retention.
	if stdoutPath, stderrPath, ok := resolveArchiveConventionPaths(jobID); ok {
		return stdoutPath, stderrPath, nil
	}

	return "", "", fmt.Errorf("could not resolve logs (job may be purged from sacct or WorkDir unavailable); also checked archive convention in %s", logArchiveDir())
}

var (
	outputFlagRe = regexp.MustCompile(`(?i)(?:^|\s)(-o|--output)\s*=?\s*(\S+)`)
	errorFlagRe  = regexp.MustCompile(`(?i)(?:^|\s)(-e|--error)\s*=?\s*(\S+)`)
	chdirFlagRe  = regexp.MustCompile(`(?i)(?:^|\s)(-D|--chdir)\s*=?\s*(\S+)`)
)

type sbatchDirectives struct {
	stdout string
	stderr string
	chdir  string
}

func parseSubmitLineDirectives(submitLine string) sbatchDirectives {
	return sbatchDirectives{
		stdout: parseFlagValue(submitLine, outputFlagRe),
		stderr: parseFlagValue(submitLine, errorFlagRe),
		chdir:  parseFlagValue(submitLine, chdirFlagRe),
	}
}

func parseSubmitLineScriptPath(submitLine string) string {
	fields := strings.Fields(submitLine)
	if len(fields) == 0 {
		return ""
	}

	start := 0
	for i, field := range fields {
		if strings.Contains(field, "sbatch") {
			start = i + 1
			break
		}
	}

	for i := start; i < len(fields); i++ {
		field := fields[i]
		if strings.HasPrefix(field, "-") {
			if !strings.Contains(field, "=") {
				if i+1 < len(fields) && submitLineFlagTakesValue(field) {
					i++
				}
			}
			continue
		}
		return strings.Trim(field, "\"'")
	}

	return ""
}

func submitLineFlagTakesValue(flag string) bool {
	switch flag {
	case "-o", "--output", "-e", "--error", "-D", "--chdir", "-J", "--job-name", "-A", "--account", "--wrap":
		return true
	default:
		return false
	}
}

func readSbatchDirectives(path string) (sbatchDirectives, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sbatchDirectives{}, err
	}
	return parseSbatchDirectives(string(data)), nil
}

func parseSbatchDirectives(contents string) sbatchDirectives {
	directives := sbatchDirectives{}
	lines := strings.Split(contents, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#SBATCH") {
			continue
		}
		if directives.stdout == "" {
			directives.stdout = parseFlagValue(trimmed, outputFlagRe)
		}
		if directives.stderr == "" {
			directives.stderr = parseFlagValue(trimmed, errorFlagRe)
		}
		if directives.chdir == "" {
			directives.chdir = parseFlagValue(trimmed, chdirFlagRe)
		}
	}
	return directives
}

func parseFlagValue(text string, re *regexp.Regexp) string {
	matches := re.FindStringSubmatch(text)
	if len(matches) < 3 {
		return ""
	}
	return cleanSbatchValue(matches[2])
}

func cleanSbatchValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	if idx := strings.IndexAny(value, "\n\r"); idx != -1 {
		value = value[:idx]
	}
	if idx := strings.Index(value, "|"); idx != -1 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func resolveLogPath(value, baseDir, jobID, jobName string) string {
	value = cleanSbatchValue(value)
	if value == "" {
		return ""
	}

	value = strings.ReplaceAll(value, "%j", jobID)
	if jobName != "" {
		value = strings.ReplaceAll(value, "%x", jobName)
	}

	if value != "" && !strings.HasPrefix(value, "/") && baseDir != "" {
		value = fmt.Sprintf("%s/%s", baseDir, value)
	}

	return value
}

func logArchiveDir() string {
	if configured := strings.TrimSpace(os.Getenv("SLURM_DASHBOARD_LOG_ARCHIVE_DIR")); configured != "" {
		return expandHomePath(configured)
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".slurm-dashboard", "logs")
}

func expandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func resolveArchiveConventionPaths(jobID string) (string, string, bool) {
	root := logArchiveDir()
	if root == "" || jobID == "" {
		return "", "", false
	}

	stdoutCandidates := []string{
		filepath.Join(root, jobID+".out"),
		filepath.Join(root, "slurm-"+jobID+".out"),
		filepath.Join(root, jobID, "stdout.log"),
		filepath.Join(root, jobID, "out.log"),
	}
	stderrCandidates := []string{
		filepath.Join(root, jobID+".err"),
		filepath.Join(root, "slurm-"+jobID+".err"),
		filepath.Join(root, jobID, "stderr.log"),
		filepath.Join(root, jobID, "err.log"),
	}

	stdoutPath := firstExistingFile(stdoutCandidates)
	stderrPath := firstExistingFile(stderrCandidates)

	// Support merged stdout/stderr.
	if stdoutPath == "" && stderrPath != "" {
		stdoutPath = stderrPath
	}
	if stderrPath == "" && stdoutPath != "" {
		stderrPath = stdoutPath
	}

	if stdoutPath == "" && stderrPath == "" {
		return "", "", false
	}
	return stdoutPath, stderrPath, true
}

func firstExistingFile(candidates []string) string {
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
