package main

import (
	"strings"
	"testing"
)

func TestFormatInsightLogSummaryOmitsDetailsAndCommands(t *testing.T) {
	raw := `{
		"status": "Critical",
		"summary": "Disk pressure detected",
		"detailedAnalysis": "Sensitive path /home/alice/private is full",
		"recommendedActions": [
			{"id":"cleanup","command":"rm -rf /tmp/cache","description":"clear cache","isSafe":false}
		]
	}`

	got := formatInsightLogSummary(raw)

	if !strings.Contains(got, "Critical") || !strings.Contains(got, "Disk pressure detected") || !strings.Contains(got, "actions=1") {
		t.Fatalf("expected compact status, summary, and action count, got %q", got)
	}
	for _, sensitive := range []string{"/home/alice/private", "rm -rf /tmp/cache"} {
		if strings.Contains(got, sensitive) {
			t.Fatalf("expected log summary to omit %q, got %q", sensitive, got)
		}
	}
}

func TestFormatInsightLogSummaryHandlesInvalidJSON(t *testing.T) {
	got := formatInsightLogSummary("plain text with command rm -rf /")

	if got != "AI insight generated" {
		t.Fatalf("expected generic fallback for invalid JSON, got %q", got)
	}
}
