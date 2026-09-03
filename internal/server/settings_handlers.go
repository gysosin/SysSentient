package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"sys-sentient/internal/config"
)

// handleGetRuntimeSettings returns the settings that can be changed live.
func (s *Server) handleGetRuntimeSettings(w http.ResponseWriter, _ *http.Request) {
	if s.runtime == nil {
		http.Error(w, "runtime settings are not available in this mode", http.StatusNotFound)
		return
	}
	writeJSONStatus(w, http.StatusOK, s.runtime.Values())
}

// handleUpdateRuntimeSettings applies a partial change.
//
// Admin-only: the poll interval controls how much load the daemon puts on the
// host it monitors, and retention controls how much history exists at all.
// Neither is a viewer's decision.
func (s *Server) handleUpdateRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, "runtime settings are not available in this mode", http.StatusNotFound)
		return
	}

	var update config.RuntimeUpdate
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	// Reject unknown fields rather than silently ignoring them: a client
	// sending `poll_interval` instead of `poll_interval_seconds` would
	// otherwise get a 200 and no change, which is the worst possible answer.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&update); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	applied, err := s.runtime.Apply(update)
	if err != nil {
		// The validation messages name the offending value and its bounds, so
		// they are safe and useful to return verbatim.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Configuration changes are worth an audit line: "why did sampling change
	// at 3am" should be answerable from the log.
	slog.Info("runtime settings updated",
		"poll_interval_seconds", applied.PollIntervalSeconds,
		"metrics_retention_hours", applied.MetricsRetentionHours,
		"minute_rollup_days", applied.MinuteRollupDays,
		"five_minute_rollup_days", applied.FiveMinuteRollupDays,
		"log_level", applied.LogLevel,
	)

	writeJSONStatus(w, http.StatusOK, applied)
}
