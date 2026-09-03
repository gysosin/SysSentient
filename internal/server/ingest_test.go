package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/alerting"
	"sys-sentient/internal/config"
	"sys-sentient/internal/models"
	"sys-sentient/internal/storage"
)

func ingestServer(t *testing.T) (*Server, *storage.Store) {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "ingest.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	srv := NewServer(config.ServerConfig{}, testPrivacy(), store, nil,
		alerting.NewEvaluator(alerting.DefaultRules()), nil)
	return srv, store
}

func postIngest(t *testing.T, srv *Server, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleIngest(rec, req)
	return rec
}

func TestIngestStoresSamplesAndRegistersHost(t *testing.T) {
	srv, store := ingestServer(t)
	now := time.Now()

	rec := postIngest(t, srv, IngestRequest{
		AgentVersion: "v0.2.0",
		Samples: []models.SystemState{
			{HostID: "remote-1", Hostname: "web-01", Timestamp: now.Add(-time.Minute), CPUUsage: 10},
			{HostID: "remote-1", Hostname: "web-01", Timestamp: now, CPUUsage: 20},
		},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/ingest = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var resp IngestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Accepted != 2 || resp.Rejected != 0 {
		t.Fatalf("accepted=%d rejected=%d, want 2/0", resp.Accepted, resp.Rejected)
	}

	// Samples must be attributable to the remote host, not the server.
	samples, err := store.GetRecentForHost("remote-1", 10)
	if err != nil {
		t.Fatalf("GetRecentForHost() error = %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("stored %d samples for remote-1, want 2", len(samples))
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("registered %d hosts, want 1", len(hosts))
	}
	if hosts[0].Hostname != "web-01" || hosts[0].AgentVersion != "v0.2.0" {
		t.Fatalf("host record = %+v, want web-01 running v0.2.0", hosts[0])
	}
}

func TestIngestRejectsUnattributableAndImplausibleSamples(t *testing.T) {
	srv, store := ingestServer(t)
	now := time.Now()

	rec := postIngest(t, srv, IngestRequest{
		Samples: []models.SystemState{
			{HostID: "", Hostname: "nameless", Timestamp: now},                // no identity
			{HostID: "ok", Hostname: "h", Timestamp: time.Time{}},             // zero clock
			{HostID: "ok", Hostname: "h", Timestamp: now.Add(24 * time.Hour)}, // clock far ahead
			{HostID: "ok", Hostname: "h", Timestamp: now, CPUUsage: 5},        // good
		},
	})

	var resp IngestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Accepted != 1 {
		t.Fatalf("accepted = %d, want 1", resp.Accepted)
	}
	if resp.Rejected != 3 {
		t.Fatalf("rejected = %d, want 3", resp.Rejected)
	}

	all, err := store.GetRecent(50)
	if err != nil {
		t.Fatalf("GetRecent() error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("stored %d samples, want only the valid one", len(all))
	}
}

func TestIngestRejectsEmptyAndOversizedBatches(t *testing.T) {
	srv, _ := ingestServer(t)

	if rec := postIngest(t, srv, IngestRequest{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty batch = %d, want 400", rec.Code)
	}

	oversized := make([]models.SystemState, maxIngestBatch+1)
	for i := range oversized {
		oversized[i] = models.SystemState{HostID: "h", Timestamp: time.Now()}
	}
	rec := postIngest(t, srv, IngestRequest{Samples: oversized})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized batch = %d, want 413", rec.Code)
	}
}

func TestIngestRejectsMalformedPayload(t *testing.T) {
	srv, _ := ingestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader([]byte(`{"samples": "not-an-array"}`)))
	rec := httptest.NewRecorder()
	srv.handleIngest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed payload = %d, want 400", rec.Code)
	}
}

func TestIngestEvaluatesAlertsOnlyForTheNewestSample(t *testing.T) {
	// An agent replaying a backlog after a partition must not fire an alert per
	// historical sample.
	srv, store := ingestServer(t)
	now := time.Now()

	// A rule that fires immediately keeps the assertion simple.
	srv.evaluator.ReplaceRules([]alerting.Rule{{
		ID: "cpu", Name: "CPU high", Metric: alerting.MetricCPUUsage,
		Op: alerting.GreaterThan, Threshold: 50, Severity: alerting.SeverityWarning, Enabled: true,
	}})

	backlog := make([]models.SystemState, 0, 10)
	for i := 0; i < 10; i++ {
		backlog = append(backlog, models.SystemState{
			HostID: "h1", Hostname: "web-01",
			Timestamp: now.Add(time.Duration(-10+i) * time.Minute),
			CPUUsage:  95,
		})
	}

	if rec := postIngest(t, srv, IngestRequest{Samples: backlog}); rec.Code != http.StatusOK {
		t.Fatalf("ingest = %d, want 200", rec.Code)
	}

	events, err := store.GetRecentAlertEvents(50)
	if err != nil {
		t.Fatalf("GetRecentAlertEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("a 10-sample backlog produced %d alert events, want 1", len(events))
	}
}

func TestHostsEndpointListsFleet(t *testing.T) {
	srv, _ := ingestServer(t)
	now := time.Now()

	postIngest(t, srv, IngestRequest{AgentVersion: "v1", Samples: []models.SystemState{
		{HostID: "a", Hostname: "web-01", Timestamp: now},
	}})
	postIngest(t, srv, IngestRequest{AgentVersion: "v1", Samples: []models.SystemState{
		{HostID: "b", Hostname: "db-01", Timestamp: now},
	}})

	rec := httptest.NewRecorder()
	srv.handleHosts(rec, httptest.NewRequest(http.MethodGet, "/api/hosts", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/hosts = %d, want 200", rec.Code)
	}
	var hosts []storage.Host
	if err := json.Unmarshal(rec.Body.Bytes(), &hosts); err != nil {
		t.Fatalf("hosts response not JSON: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("listed %d hosts, want 2", len(hosts))
	}
}

func TestMetricsEndpointScopesByHost(t *testing.T) {
	srv, _ := ingestServer(t)
	now := time.Now()

	postIngest(t, srv, IngestRequest{Samples: []models.SystemState{
		{HostID: "web", Hostname: "web-01", Timestamp: now.Add(-2 * time.Minute), CPUUsage: 11},
		{HostID: "db", Hostname: "db-01", Timestamp: now.Add(-time.Minute), CPUUsage: 22},
		{HostID: "web", Hostname: "web-01", Timestamp: now, CPUUsage: 33},
	}})

	read := func(query string) []models.SystemState {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.handleMetrics(rec, httptest.NewRequest(http.MethodGet, "/api/metrics"+query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/metrics%s = %d, want 200", query, rec.Code)
		}
		var out []models.SystemState
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("response not JSON: %v", err)
		}
		return out
	}

	web := read("?host=web")
	if len(web) != 2 {
		t.Fatalf("?host=web returned %d samples, want 2", len(web))
	}
	for _, sample := range web {
		if sample.HostID != "web" {
			t.Fatalf("?host=web leaked a sample from %q", sample.HostID)
		}
	}

	db := read("?host=db")
	if len(db) != 1 || db[0].HostID != "db" {
		t.Fatalf("?host=db returned %+v, want a single db sample", db)
	}

	// No host param means every host, preserving single-node behaviour.
	if all := read(""); len(all) != 3 {
		t.Fatalf("unscoped query returned %d samples, want 3", len(all))
	}

	// An unknown host is empty, not everything.
	if none := read("?host=nope"); len(none) != 0 {
		t.Fatalf("unknown host returned %d samples, want 0", len(none))
	}
}

func TestMetricsEndpointHonoursLimit(t *testing.T) {
	srv, _ := ingestServer(t)
	now := time.Now()

	samples := make([]models.SystemState, 0, 20)
	for i := 0; i < 20; i++ {
		samples = append(samples, models.SystemState{
			HostID: "h", Hostname: "h", Timestamp: now.Add(time.Duration(-i) * time.Second), CPUUsage: float64(i),
		})
	}
	postIngest(t, srv, IngestRequest{Samples: samples})

	rec := httptest.NewRecorder()
	srv.handleMetrics(rec, httptest.NewRequest(http.MethodGet, "/api/metrics?limit=5", nil))

	var out []models.SystemState
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("?limit=5 returned %d samples, want 5", len(out))
	}
}
