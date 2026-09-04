package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"sys-sentient/internal/alerting"
	"sys-sentient/internal/storage"
)

// maxMuteHours caps how long an alert can be silenced.
//
// A mute with no end is a rule quietly disabled forever, discovered months
// later during an incident it should have caught.
const maxMuteHours = 30 * 24

// ruleView is a rule as the dashboard sees it: the effective values, plus
// whether they differ from the built-in defaults.
type ruleView struct {
	alerting.Rule
	ForSeconds int `json:"for_seconds"`
	// Overridden marks a rule an operator has changed, so the UI can offer to
	// restore it without guessing.
	Overridden bool       `json:"overridden"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
}

// applyOverrides layers stored operator changes onto the built-in defaults.
//
// Only differences are stored, so an install that never touches a rule follows
// the defaults as they improve rather than being pinned to whatever they were
// when it was first started.
func applyOverrides(rules []alerting.Rule, overrides map[string]storage.RuleOverride) []alerting.Rule {
	out := make([]alerting.Rule, 0, len(rules))
	for _, rule := range rules {
		if o, ok := overrides[rule.ID]; ok {
			if o.Threshold != nil {
				rule.Threshold = *o.Threshold
			}
			if o.ForSecs != nil {
				rule.For = time.Duration(*o.ForSecs) * time.Second
			}
			if o.Enabled != nil {
				rule.Enabled = *o.Enabled
			}
		}
		out = append(out, rule)
	}
	return out
}

// reloadRules re-applies stored overrides to the running evaluator.
func (s *Server) reloadRules() error {
	if s.evaluator == nil || s.store == nil {
		return nil
	}
	overrides, err := s.store.ListRuleOverrides()
	if err != nil {
		return err
	}
	s.evaluator.ReplaceRules(applyOverrides(alerting.DefaultRules(), overrides))
	return nil
}

// handleListRules returns the effective rules and which are muted.
func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	if s.evaluator == nil {
		setProtectedJSONHeaders(w)
		writeJSONBody(w, []ruleView{})
		return
	}

	overrides := map[string]storage.RuleOverride{}
	if s.store != nil {
		if stored, err := s.store.ListRuleOverrides(); err == nil {
			overrides = stored
		}
	}

	rules := s.evaluator.Rules()
	views := make([]ruleView, 0, len(rules))
	for _, rule := range rules {
		v := ruleView{Rule: rule, ForSeconds: int(rule.For.Seconds())}
		if o, ok := overrides[rule.ID]; ok {
			v.Overridden = true
			v.MutedUntil = o.MutedUntil
		}
		views = append(views, v)
	}

	setProtectedJSONHeaders(w)
	writeJSONBody(w, views)
}

type ruleUpdateRequest struct {
	Threshold *float64 `json:"threshold"`
	ForSecs   *int     `json:"for_seconds"`
	Enabled   *bool    `json:"enabled"`
	// MuteHours silences notifications for a bounded time. Zero clears a mute.
	MuteHours *float64 `json:"mute_hours"`
}

// handleUpdateRule changes one rule.
//
// Rules were hardcoded and read-only: Rule.Enabled was honoured by the
// evaluator but nothing could ever set it false, and thresholds could not be
// tuned without editing the source and rebuilding.
func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	if s.store == nil || s.evaluator == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alerting is not enabled")
		return
	}

	ruleID := r.PathValue("ruleID")
	var known bool
	var base alerting.Rule
	for _, rule := range alerting.DefaultRules() {
		if rule.ID == ruleID {
			base, known = rule, true
			break
		}
	}
	if !known {
		writeJSONError(w, http.StatusNotFound, "no such rule")
		return
	}

	var req ruleUpdateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate against the rule the change would produce, so a threshold and a
	// duration that are individually plausible but jointly invalid are caught.
	candidate := base
	if req.Threshold != nil {
		candidate.Threshold = *req.Threshold
	}
	if req.ForSecs != nil {
		if *req.ForSecs < 0 || *req.ForSecs > 24*3600 {
			writeJSONError(w, http.StatusBadRequest, "for_seconds must be between 0 and 86400")
			return
		}
		candidate.For = time.Duration(*req.ForSecs) * time.Second
	}
	if req.Enabled != nil {
		candidate.Enabled = *req.Enabled
	}
	if err := candidate.Validate(); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, _ := s.store.ListRuleOverrides()
	override := existing[ruleID]
	override.RuleID = ruleID
	if req.Threshold != nil {
		override.Threshold = req.Threshold
	}
	if req.ForSecs != nil {
		override.ForSecs = req.ForSecs
	}
	if req.Enabled != nil {
		override.Enabled = req.Enabled
	}
	if req.MuteHours != nil {
		if *req.MuteHours < 0 || *req.MuteHours > maxMuteHours {
			writeJSONError(w, http.StatusBadRequest, "mute_hours must be between 0 and 720")
			return
		}
		if *req.MuteHours == 0 {
			override.MutedUntil = nil
		} else {
			until := time.Now().Add(time.Duration(*req.MuteHours * float64(time.Hour)))
			override.MutedUntil = &until
		}
	}

	p, _ := principalFrom(r.Context())
	override.UpdatedAt = time.Now()
	override.UpdatedBy = p.user.Email

	if err := s.store.SaveRuleOverride(override); err != nil {
		slog.Error("save rule override", "rule", ruleID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not save the rule")
		return
	}
	if err := s.reloadRules(); err != nil {
		slog.Error("reload rules", "error", err)
	}

	slog.Info("alert rule changed", "rule", ruleID, "by", p.user.Email,
		"muted_until", override.MutedUntil)
	s.handleListRules(w, r)
}

// handleResetRule restores a rule to its built-in defaults.
func (s *Server) handleResetRule(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alerting is not enabled")
		return
	}
	err := s.store.DeleteRuleOverride(r.PathValue("ruleID"))
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		writeJSONError(w, http.StatusInternalServerError, "could not reset the rule")
		return
	}
	if err := s.reloadRules(); err != nil {
		slog.Error("reload rules", "error", err)
	}
	s.handleListRules(w, r)
}

// mutedRules reports which rules are currently silenced.
func (s *Server) mutedRules(now time.Time) map[string]bool {
	muted := map[string]bool{}
	if s.store == nil {
		return muted
	}
	overrides, err := s.store.ListRuleOverrides()
	if err != nil {
		return muted
	}
	for id, o := range overrides {
		if o.Muted(now) {
			muted[id] = true
		}
	}
	return muted
}

// ReloadRules re-applies stored operator changes. Called at startup so a
// restart does not silently revert them.
func (s *Server) ReloadRules() error { return s.reloadRules() }
