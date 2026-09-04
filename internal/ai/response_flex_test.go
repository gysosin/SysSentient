package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// decodeNormalized runs the normaliser and unpacks its output.
func decodeNormalized(t *testing.T, raw string) map[string]any {
	t.Helper()
	out, err := NormalizeAnalysisResponse(raw)
	if err != nil {
		t.Fatalf("NormalizeAnalysisResponse: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("normalised output is not JSON: %v", err)
	}
	return got
}

func TestDetailedAnalysisAcceptsArray(t *testing.T) {
	// What Gemini actually returned against a live key: the prompt asks for
	// bullet points, so the model produced an array of them. This failed the
	// whole analysis with "cannot unmarshal array into Go struct field".
	raw := `{
		"status": "Warning",
		"summary": "Swap is under pressure.",
		"detailedAnalysis": [
			"Swap is 90% used while memory sits at 43%.",
			"Chrome is holding 1149 MB across several processes."
		],
		"recommendedActions": []
	}`

	got := decodeNormalized(t, raw)
	detail, ok := got["detailedAnalysis"].(string)
	if !ok {
		t.Fatalf("detailedAnalysis = %T, want string", got["detailedAnalysis"])
	}
	// Every bullet must survive; dropping one silently loses analysis.
	for _, want := range []string{"90% used", "1149 MB"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detailedAnalysis lost %q: %s", want, detail)
		}
	}
	if !strings.Contains(detail, "\n") {
		t.Errorf("bullets were run together instead of separated: %q", detail)
	}
}

func TestDetailedAnalysisStillAcceptsString(t *testing.T) {
	raw := `{"status":"Healthy","summary":"All good.","detailedAnalysis":"Nothing to report.","recommendedActions":[]}`
	got := decodeNormalized(t, raw)
	if got["detailedAnalysis"] != "Nothing to report." {
		t.Errorf("detailedAnalysis = %v", got["detailedAnalysis"])
	}
}

func TestDetailedAnalysisAcceptsNestedObjects(t *testing.T) {
	// Models also return arrays of objects when asked for structured points.
	raw := `{
		"status": "Warning",
		"summary": "Memory pressure.",
		"detailedAnalysis": [
			{"point": "Swap at 90%"},
			{"point": "Chrome dominant"}
		],
		"recommendedActions": []
	}`
	got := decodeNormalized(t, raw)
	detail, ok := got["detailedAnalysis"].(string)
	if !ok {
		t.Fatalf("detailedAnalysis = %T, want string", got["detailedAnalysis"])
	}
	if !strings.Contains(detail, "Swap at 90%") || !strings.Contains(detail, "Chrome dominant") {
		t.Errorf("nested objects lost their content: %q", detail)
	}
}

func TestSummaryAcceptsArrayToo(t *testing.T) {
	// The same class of failure applies to any free-text field.
	raw := `{"status":"Warning","summary":["First.","Second."],"detailedAnalysis":"x","recommendedActions":[]}`
	got := decodeNormalized(t, raw)
	summary, ok := got["summary"].(string)
	if !ok || !strings.Contains(summary, "First.") {
		t.Errorf("summary = %v, want the array joined into a string", got["summary"])
	}
}

func TestMalformedJSONStillFails(t *testing.T) {
	// Tolerance must not extend to accepting genuine garbage.
	if _, err := NormalizeAnalysisResponse("{not json"); err == nil {
		t.Fatal("NormalizeAnalysisResponse accepted malformed JSON")
	}
}
