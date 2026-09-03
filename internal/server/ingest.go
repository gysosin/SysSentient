package server

import (
	gocontext "context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"sys-sentient/internal/models"
	"sys-sentient/internal/storage"
)

// maxIngestBytes bounds a single push. At ~4 KB per sample a 60-sample batch is
// well under this; the limit exists to stop an authenticated-but-buggy agent
// from exhausting server memory.
const maxIngestBytes = 8 << 20 // 8 MiB

// maxIngestBatch bounds how many samples one request may carry.
const maxIngestBatch = 1000

// IngestRequest is what an agent pushes.
type IngestRequest struct {
	AgentVersion string               `json:"agent_version"`
	Samples      []models.SystemState `json:"samples"`
}

// IngestResponse reports what the server accepted.
type IngestResponse struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

// handleIngest accepts a batch of samples pushed by an agent.
//
// This is the write path that made multi-host possible: previously the only way
// data entered the system was the in-process collect loop, so a second machine
// could not report at all.
//
// Authenticated by the agent key, which is deliberately distinct from the
// dashboard API key — the dashboard key is readable by anyone who can load the
// page, and must never confer write access.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBytes)

	var req IngestRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid ingest payload")
		return
	}

	if len(req.Samples) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no samples supplied")
		return
	}
	if len(req.Samples) > maxIngestBatch {
		writeJSONError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("batch too large: %d samples (max %d)", len(req.Samples), maxIngestBatch))
		return
	}

	var accepted, rejected int
	seenHosts := make(map[string]models.SystemState, 1)

	for _, sample := range req.Samples {
		// A sample with no host identity cannot be attributed, and storing it
		// would silently pollute every host-scoped query.
		if sample.HostID == "" {
			rejected++
			continue
		}
		// Reject implausible timestamps rather than letting a agent with a
		// broken clock poison retention and ordering.
		if sample.Timestamp.IsZero() || sample.Timestamp.After(time.Now().Add(15*time.Minute)) {
			rejected++
			continue
		}

		if err := s.store.Save(&sample); err != nil {
			slog.Error("failed to store ingested sample", "host_id", sample.HostID, "error", err)
			rejected++
			continue
		}
		accepted++
		seenHosts[sample.HostID] = sample
	}

	// Register the hosts and evaluate alerts against their newest sample only:
	// a backlog replayed after a network partition must not fire an alert for
	// every historical sample it contains.
	for hostID, newest := range seenHosts {
		if err := s.store.UpsertHost(hostID, newest.Hostname, req.AgentVersion, time.Now()); err != nil {
			slog.Error("failed to record host", "host_id", hostID, "error", err)
		}
		s.evaluateAndRecord(newest)
	}

	setProtectedJSONHeaders(w)
	if accepted == 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	writeJSONBody(w, IngestResponse{Accepted: accepted, Rejected: rejected})
}

// evaluateAndRecord runs alert rules for one sample and persists any
// transitions. Shared by the ingest path and the local collector loop so both
// behave identically.
func (s *Server) evaluateAndRecord(state models.SystemState) {
	if s.evaluator == nil {
		return
	}

	now := time.Now()
	transitions := s.evaluator.Evaluate(state, now)
	if len(transitions) == 0 {
		return
	}

	for _, transition := range transitions {
		slog.Warn("alert transition",
			"state", string(transition.State),
			"rule", transition.RuleID,
			"host", transition.Hostname,
			"value", transition.Value,
		)
		if s.store != nil {
			if err := s.store.SaveAlertEvent(storage.AlertEvent{
				OccurredAt: now,
				RuleID:     transition.RuleID,
				RuleName:   transition.RuleName,
				Metric:     string(transition.Metric),
				State:      string(transition.State),
				Severity:   string(transition.Severity),
				Value:      transition.Value,
				Threshold:  transition.Threshold,
				Hostname:   transition.Hostname,
			}); err != nil {
				slog.Error("failed to store alert event", "error", err)
			}
		}
	}

	if s.dispatcher != nil {
		go s.dispatcher.Dispatch(gocontext.Background(), transitions)
	}
}

// handleHosts returns the fleet inventory.
func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		setProtectedJSONHeaders(w)
		writeJSONBody(w, []storage.Host{})
		return
	}

	hosts, err := s.store.ListHosts()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list hosts")
		return
	}

	setProtectedJSONHeaders(w)
	writeJSONBody(w, hosts)
}
