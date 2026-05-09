package ai

import (
	"encoding/json"
	"testing"

	"sys-sentient/internal/models"
)

func TestNormalizeAnalysisResponseDefaultsSafely(t *testing.T) {
	raw := `{
		"status": "Unknown",
		"summary": " ",
		"detailedAnalysis": "",
		"recommendedActions": [
			{"command": " systemctl status ssh ", "description": " inspect service "},
			{"command": "", "description": ""}
		]
	}`

	normalized, err := NormalizeAnalysisResponse(raw)
	if err != nil {
		t.Fatalf("expected normalization to succeed: %v", err)
	}

	var analysis models.AIAnalysis
	if err := json.Unmarshal([]byte(normalized), &analysis); err != nil {
		t.Fatalf("expected normalized JSON: %v", err)
	}

	if analysis.Status != "Warning" {
		t.Fatalf("expected invalid status to default to Warning, got %q", analysis.Status)
	}
	if analysis.Summary != "AI Analysis Generated" {
		t.Fatalf("expected fallback summary, got %q", analysis.Summary)
	}
	if analysis.DetailedAnalysis != analysis.Summary {
		t.Fatalf("expected detailed analysis fallback, got %q", analysis.DetailedAnalysis)
	}
	if len(analysis.RecommendedActions) != 1 {
		t.Fatalf("expected one useful action, got %d", len(analysis.RecommendedActions))
	}
	action := analysis.RecommendedActions[0]
	if action.ID != "action_1" {
		t.Fatalf("expected generated action ID, got %q", action.ID)
	}
	if action.IsSafe {
		t.Fatal("expected unspecified action safety to default to restricted")
	}
}

func TestNormalizeAnalysisResponseRejectsInvalidJSON(t *testing.T) {
	if _, err := NormalizeAnalysisResponse("plain text"); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}
