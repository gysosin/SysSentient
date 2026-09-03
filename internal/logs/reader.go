package logs

import (
	"bufio"
	"context"
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

// logSource is one place this platform keeps system logs. Each platform file
// supplies its own list; everything downstream — filtering, truncation, the
// PII scrubber, the AI prompt — is shared.
type logSource struct {
	// heading labels the block in the combined output.
	heading string
	fetch   func(r *LogReader, ctx context.Context) (string, error)
}

// GetRecentLogsContext collects recent logs from every source this platform
// offers.
//
// Sources fail soft on purpose: journalctl may be absent, dmesg needs
// privileges the hardened systemd unit deliberately drops, and the Windows
// event log may be unreadable for the service account. One unavailable source
// must not cost the others, and no source at all is a valid answer rather than
// an error — a machine with nothing to report is the common case.
func (r *LogReader) GetRecentLogsContext(ctx context.Context) (string, error) {
	var allLogs strings.Builder

	for _, src := range platformLogSources {
		out, err := src.fetch(r, ctx)
		if err != nil || out == "" {
			continue
		}
		allLogs.WriteString("=== " + src.heading + " ===\n")
		allLogs.WriteString(out)
		allLogs.WriteString("\n")
	}

	result := allLogs.String()
	if result == "" {
		return "No recent errors or warnings found in system logs.", nil
	}

	return result, nil
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
