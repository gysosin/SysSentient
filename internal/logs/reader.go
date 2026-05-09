package logs

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultCommandTimeout = 1500 * time.Millisecond

type commandRunner func(context.Context, string, ...string) ([]byte, error)

// LogReader collects system logs from various sources
type LogReader struct {
	maxLines       int
	commandTimeout time.Duration
	runCommand     commandRunner
}

// NewLogReader creates a new log reader
func NewLogReader(maxLines int) *LogReader {
	if maxLines <= 0 {
		maxLines = 100
	}
	return &LogReader{
		maxLines:       maxLines,
		commandTimeout: defaultCommandTimeout,
		runCommand:     runCommand,
	}
}

// LogEntry represents a single log line
type LogEntry struct {
	Timestamp time.Time
	Source    string // journalctl, dmesg, syslog
	Level     string // ERROR, WARN, INFO
	Message   string
}

// GetRecentLogs collects recent logs from multiple sources
func (r *LogReader) GetRecentLogs() (string, error) {
	return r.GetRecentLogsContext(context.Background())
}

// GetRecentLogsContext collects recent logs from multiple sources.
func (r *LogReader) GetRecentLogsContext(ctx context.Context) (string, error) {
	var allLogs strings.Builder

	// 1. Get journalctl errors (systemd journal)
	journalLogs, err := r.getJournalErrors(ctx)
	if err == nil && journalLogs != "" {
		allLogs.WriteString("=== SYSTEMD JOURNAL (Errors/Warnings) ===\n")
		allLogs.WriteString(journalLogs)
		allLogs.WriteString("\n")
	}

	// 2. Get dmesg errors (kernel ring buffer)
	dmesgLogs, err := r.getDmesgErrors(ctx)
	if err == nil && dmesgLogs != "" {
		allLogs.WriteString("=== KERNEL MESSAGES (Errors/Warnings) ===\n")
		allLogs.WriteString(dmesgLogs)
		allLogs.WriteString("\n")
	}

	// 3. Get syslog errors if available
	syslogLogs, err := r.getSyslogErrors(ctx)
	if err == nil && syslogLogs != "" {
		allLogs.WriteString("=== SYSLOG (Recent Errors) ===\n")
		allLogs.WriteString(syslogLogs)
		allLogs.WriteString("\n")
	}

	result := allLogs.String()
	if result == "" {
		return "No recent errors or warnings found in system logs.", nil
	}

	return result, nil
}

// getJournalErrors gets recent error/warning entries from journalctl
func (r *LogReader) getJournalErrors(ctx context.Context) (string, error) {
	// Get logs from last 10 minutes with priority error or warning
	output, err := r.runLogCommand(ctx, "journalctl", "--since", "10 minutes ago", "-p", "warning", "--no-pager", "-n", fmt.Sprintf("%d", r.maxLines))
	if err != nil {
		// journalctl might not be available or user lacks permissions
		return "", fmt.Errorf("journalctl failed: %w", err)
	}

	return r.filterRelevantLines(string(output)), nil
}

// getDmesgErrors gets recent kernel errors from dmesg
func (r *LogReader) getDmesgErrors(ctx context.Context) (string, error) {
	output, err := r.runLogCommand(ctx, "dmesg", "-l", "err,warn", "-T")
	if err != nil {
		// dmesg might require root or not available
		return "", fmt.Errorf("dmesg failed: %w", err)
	}

	lines := r.getTailLines(string(output), r.maxLines)
	return r.filterRelevantLines(lines), nil
}

// getSyslogErrors reads recent errors from /var/log/syslog
func (r *LogReader) getSyslogErrors(ctx context.Context) (string, error) {
	// Try to read syslog with tail
	output, err := r.runLogCommand(ctx, "tail", "-n", fmt.Sprintf("%d", r.maxLines), "/var/log/syslog")
	if err != nil {
		// syslog might not exist or permission denied
		return "", fmt.Errorf("syslog read failed: %w", err)
	}

	// Filter for error/warning keywords
	return r.filterErrorLines(string(output)), nil
}

// filterRelevantLines removes empty lines and truncates
func (r *LogReader) filterRelevantLines(input string) string {
	var result strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(input))
	lineCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && lineCount < r.maxLines {
			result.WriteString(line)
			result.WriteString("\n")
			lineCount++
		}
	}

	return result.String()
}

// filterErrorLines only keeps lines with error/warning keywords
func (r *LogReader) filterErrorLines(input string) string {
	var result strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(input))
	lineCount := 0

	errorKeywords := []string{"error", "err", "warn", "warning", "fail", "critical", "panic"}

	for scanner.Scan() {
		line := scanner.Text()
		lineLower := strings.ToLower(line)

		for _, keyword := range errorKeywords {
			if strings.Contains(lineLower, keyword) && lineCount < r.maxLines {
				result.WriteString(line)
				result.WriteString("\n")
				lineCount++
				break
			}
		}
	}

	return result.String()
}

// getTailLines gets the last N lines from a string
func (r *LogReader) getTailLines(input string, n int) string {
	lines := strings.Split(input, "\n")

	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}

	return strings.Join(lines[start:], "\n")
}

// GetLogsWithTimeout collects logs with a timeout to prevent hanging
func (r *LogReader) GetLogsWithTimeout(timeout time.Duration) (string, error) {
	return r.GetLogsContextWithTimeout(context.Background(), timeout)
}

// GetLogsContextWithTimeout collects logs with a timeout derived from a parent context.
func (r *LogReader) GetLogsContextWithTimeout(ctx context.Context, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return r.GetRecentLogsContext(ctx)
}

func (r *LogReader) runLogCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	commandTimeout := r.commandTimeout
	if commandTimeout <= 0 {
		commandTimeout = defaultCommandTimeout
	}

	cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	runner := r.runCommand
	if runner == nil {
		runner = runCommand
	}

	output, err := runner(cmdCtx, name, args...)
	if cmdCtx.Err() != nil {
		return nil, cmdCtx.Err()
	}
	return output, err
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
