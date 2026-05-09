package main

import (
	"strings"
	"sys-sentient/internal/models"
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

func TestShouldTriggerAutomaticAnalysis(t *testing.T) {
	tests := []struct {
		name  string
		state models.SystemState
		want  bool
	}{
		{
			name:  "high cpu triggers",
			state: models.SystemState{CPUUsage: 81, MemoryUsed: 1, MemoryTotal: 100},
			want:  true,
		},
		{
			name:  "high memory triggers",
			state: models.SystemState{CPUUsage: 10, MemoryUsed: 91, MemoryTotal: 100},
			want:  true,
		},
		{
			name:  "zero memory total does not trigger",
			state: models.SystemState{CPUUsage: 10, MemoryUsed: 1, MemoryTotal: 0},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldTriggerAutomaticAnalysis(tt.state); got != tt.want {
				t.Fatalf("shouldTriggerAutomaticAnalysis() = %v, want %v", got, tt.want)
			}
		})
	}
}
