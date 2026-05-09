package logs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewLogReader(t *testing.T) {
	reader := NewLogReader(50)
	if reader.maxLines != 50 {
		t.Errorf("Expected maxLines 50, got %d", reader.maxLines)
	}

	// Test default value
	reader = NewLogReader(0)
	if reader.maxLines != 100 {
		t.Errorf("Expected default maxLines 100, got %d", reader.maxLines)
	}
}

func TestFilterRelevantLines(t *testing.T) {
	reader := NewLogReader(5)

	input := "\nLine 1\n\nLine 2\nLine 3\n\n\nLine 4\nLine 5\nLine 6\n"

	result := reader.filterRelevantLines(input)
	lines := strings.Split(strings.TrimSpace(result), "\n")

	if len(lines) != 5 {
		t.Errorf("Expected 5 lines, got %d", len(lines))
	}
}

func TestFilterErrorLines(t *testing.T) {
	reader := NewLogReader(10)

	input := `
	This is a normal line
	ERROR: Something went wrong
	Another normal line
	WARNING: Be careful
	kernel: panic at line 42
	`

	result := reader.filterErrorLines(input)
	lines := strings.Split(strings.TrimSpace(result), "\n")

	// Should contain ERROR and WARNING lines
	if len(lines) < 2 {
		t.Errorf("Expected at least 2 error lines, got %d", len(lines))
	}
}

func TestGetTailLines(t *testing.T) {
	reader := NewLogReader(100)

	input := strings.Join([]string{"Line1", "Line2", "Line3", "Line4", "Line5"}, "\n")

	result := reader.getTailLines(input, 3)
	expected := "Line3\nLine4\nLine5"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestGetLogsWithTimeout(t *testing.T) {
	reader := NewLogReader(10)
	reader.runCommand = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte("May 09 warning: synthetic journal entry\n"), nil
		}
		return nil, errors.New("source unavailable")
	}

	// This should complete quickly
	logs, err := reader.GetLogsWithTimeout(2 * time.Second)

	// We don't expect timeout error in normal conditions
	if err != nil && strings.Contains(err.Error(), "timed out") {
		t.Error("Log collection should not timeout in normal conditions")
	}
	if !strings.Contains(logs, "SYSTEMD JOURNAL") {
		t.Fatalf("Expected journal logs, got %q", logs)
	}
}

func TestGetLogsWithTimeoutBoundsSlowSources(t *testing.T) {
	reader := NewLogReader(10)
	reader.commandTimeout = 10 * time.Millisecond
	reader.runCommand = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	start := time.Now()
	logs, err := reader.GetLogsWithTimeout(200 * time.Millisecond)
	if err != nil {
		t.Fatalf("Expected best-effort log collection, got error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Expected slow sources to be bounded, took %v", elapsed)
	}
	if !strings.Contains(logs, "No recent errors") {
		t.Fatalf("Expected empty fallback message, got %q", logs)
	}
}
