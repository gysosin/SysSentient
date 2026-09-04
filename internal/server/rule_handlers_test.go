package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sys-sentient/internal/alerting"
)

func rulesServer(t *testing.T) *Server {
	t.Helper()
	srv, _ := ingestServer(t)
	return srv
}

func patchRule(t *testing.T, srv *Server, ruleID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/alerts/rules/"+ruleID, bytes.NewReader(raw))
	req.SetPathValue("ruleID", ruleID)
	rec := httptest.NewRecorder()
	srv.handleUpdateRule(rec, req)
	return rec
}

func readRules(t *testing.T, rec *httptest.ResponseRecorder) []ruleView {
	t.Helper()
	var views []ruleView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode rules: %v (%s)", err, rec.Body.String())
	}
	return views
}

func findRule(views []ruleView, id string) *ruleView {
	for i := range views {
		if views[i].ID == id {
			return &views[i]
		}
	}
	return nil
}

func TestRuleThresholdCanBeChanged(t *testing.T) {
	srv := rulesServer(t)
	threshold := 75.0

	rec := patchRule(t, srv, "cpu-high", ruleUpdateRequest{Threshold: &threshold})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH = %d (%s)", rec.Code, rec.Body.String())
	}

	rule := findRule(readRules(t, rec), "cpu-high")
	if rule == nil || rule.Threshold != 75 {
		t.Fatalf("threshold = %+v, want 75", rule)
	}
	if !rule.Overridden {
		t.Error("rule is not marked as overridden")
	}

	// The running evaluator must see it, not just the stored row — otherwise
	// the change is cosmetic until a restart.
	for _, r := range srv.evaluator.Rules() {
		if r.ID == "cpu-high" && r.Threshold != 75 {
			t.Errorf("evaluator still has threshold %v", r.Threshold)
		}
	}
}

func TestRuleCanBeDisabled(t *testing.T) {
	srv := rulesServer(t)
	off := false

	// Rule.Enabled was honoured by the evaluator but nothing could ever set it
	// false: the endpoint was read-only and there was no PATCH.
	if rec := patchRule(t, srv, "cpu-high", ruleUpdateRequest{Enabled: &off}); rec.Code != http.StatusOK {
		t.Fatalf("PATCH = %d (%s)", rec.Code, rec.Body.String())
	}
	for _, r := range srv.evaluator.Rules() {
		if r.ID == "cpu-high" && r.Enabled {
			t.Error("rule is still enabled in the evaluator")
		}
	}
}

func TestRuleMuteSuppressesNotificationsButNotEvaluation(t *testing.T) {
	srv := rulesServer(t)
	hours := 2.0

	if rec := patchRule(t, srv, "cpu-high", ruleUpdateRequest{MuteHours: &hours}); rec.Code != http.StatusOK {
		t.Fatalf("PATCH = %d (%s)", rec.Code, rec.Body.String())
	}

	now := time.Now()
	muted := srv.mutedRules(now)
	if !muted["cpu-high"] {
		t.Fatal("cpu-high is not muted")
	}

	// Muting must not stop the alert existing — it stops it paging anyone.
	transitions := []alerting.Alert{
		{RuleID: "cpu-high", State: alerting.StateFiring},
		{RuleID: "disk-full", State: alerting.StateFiring},
	}
	notify := srv.notifiable(transitions, now)
	if len(notify) != 1 || notify[0].RuleID != "disk-full" {
		t.Fatalf("notifiable = %+v, want only disk-full", notify)
	}

	// A mute expires on its own; a silence with no end is a rule quietly
	// disabled forever.
	if srv.mutedRules(now.Add(3 * time.Hour))["cpu-high"] {
		t.Error("mute did not expire")
	}
}

func TestRuleChangesAreValidatedAndBounded(t *testing.T) {
	srv := rulesServer(t)
	tooLong := 999999
	tooManyHours := 10000.0

	for name, body := range map[string]ruleUpdateRequest{
		"absurd for":   {ForSecs: &tooLong},
		"endless mute": {MuteHours: &tooManyHours},
	} {
		t.Run(name, func(t *testing.T) {
			if rec := patchRule(t, srv, "cpu-high", body); rec.Code != http.StatusBadRequest {
				t.Errorf("PATCH = %d, want 400", rec.Code)
			}
		})
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/alerts/rules/nope", bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("ruleID", "nope")
	rec := httptest.NewRecorder()
	srv.handleUpdateRule(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown rule = %d, want 404", rec.Code)
	}
}

func TestRuleCanBeResetToItsDefault(t *testing.T) {
	srv := rulesServer(t)
	threshold := 50.0
	patchRule(t, srv, "cpu-high", ruleUpdateRequest{Threshold: &threshold})

	req := httptest.NewRequest(http.MethodDelete, "/api/alerts/rules/cpu-high", nil)
	req.SetPathValue("ruleID", "cpu-high")
	rec := httptest.NewRecorder()
	srv.handleResetRule(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE = %d (%s)", rec.Code, rec.Body.String())
	}

	rule := findRule(readRules(t, rec), "cpu-high")
	if rule == nil || rule.Overridden {
		t.Fatalf("rule still overridden: %+v", rule)
	}
	// Back to the built-in value, not to whatever was stored.
	var want float64
	for _, r := range alerting.DefaultRules() {
		if r.ID == "cpu-high" {
			want = r.Threshold
		}
	}
	if rule.Threshold != want {
		t.Errorf("threshold = %v, want the default %v", rule.Threshold, want)
	}
}
