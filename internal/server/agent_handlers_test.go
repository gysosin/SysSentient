package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/models"
)

// enrol runs the full join flow and returns the agent's own credential.
func enrol(t *testing.T, srv *Server, hostID string) joinResponse {
	t.Helper()
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	now := time.Now()
	if err := srv.store.CreateJoinToken("tok-1", auth.HashToken(token), "web-01",
		"admin@example.com", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}

	rec := postJSON(t, srv.handleAgentJoin, "/api/agents/join", joinRequest{
		Token: token, HostID: hostID, Hostname: "web-01.example.com", Version: "0.1.0",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("join = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}

	var resp joinResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode join response: %v", err)
	}
	return resp
}

func postJSON(t *testing.T, h http.HandlerFunc, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestAgentJoinReturnsUsableCredential(t *testing.T) {
	srv, _ := ingestServer(t)
	// A key must be configured, or auth is disabled and the test proves nothing.
	srv.config.AgentKey = "shared-fleet-key"
	srv.agentAuth = NewAuthMiddleware(srv.config.AgentKey)

	joined := enrol(t, srv, "remote-1")
	if joined.AgentKey == "" {
		t.Fatal("join returned an empty agent key")
	}

	// The credential it was handed must actually authenticate an ingest.
	body, _ := json.Marshal(IngestRequest{
		AgentVersion: "0.1.0",
		Samples: []models.SystemState{
			{HostID: "remote-1", Hostname: "web-01", Timestamp: time.Now(), CPUUsage: 12},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("X-API-Key", joined.AgentKey)
	rec := httptest.NewRecorder()
	srv.authenticateAgent(srv.handleIngest)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ingest with agent key = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestRevokedAgentCannotIngest(t *testing.T) {
	srv, store := ingestServer(t)
	srv.config.AgentKey = "shared-fleet-key"
	srv.agentAuth = NewAuthMiddleware(srv.config.AgentKey)

	joined := enrol(t, srv, "remote-1")
	if err := store.RevokeAgent(joined.AgentID, time.Now()); err != nil {
		t.Fatalf("RevokeAgent: %v", err)
	}

	body, _ := json.Marshal(IngestRequest{
		Samples: []models.SystemState{{HostID: "remote-1", Timestamp: time.Now()}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("X-API-Key", joined.AgentKey)
	rec := httptest.NewRecorder()
	srv.authenticateAgent(srv.handleIngest)(rec, req)

	// 403 not 401: the agent should stop retrying, not re-authenticate.
	if rec.Code != http.StatusForbidden {
		t.Fatalf("revoked ingest = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
}

func TestSharedKeyStillWorksAfterPerAgentCredentialsExist(t *testing.T) {
	srv, _ := ingestServer(t)
	srv.config.AgentKey = "shared-fleet-key"
	srv.agentAuth = NewAuthMiddleware(srv.config.AgentKey)
	enrol(t, srv, "remote-1")

	// An existing fleet on the shared key must survive the upgrade that
	// introduces per-agent credentials.
	body, _ := json.Marshal(IngestRequest{
		Samples: []models.SystemState{
			{HostID: "legacy-1", Hostname: "old", Timestamp: time.Now(), CPUUsage: 5},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "shared-fleet-key")
	rec := httptest.NewRecorder()
	srv.authenticateAgent(srv.handleIngest)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("shared-key ingest = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestIngestRejectsMissingAndWrongCredentials(t *testing.T) {
	srv, _ := ingestServer(t)
	srv.config.AgentKey = "shared-fleet-key"
	srv.agentAuth = NewAuthMiddleware(srv.config.AgentKey)

	body, _ := json.Marshal(IngestRequest{
		Samples: []models.SystemState{{HostID: "x", Timestamp: time.Now()}},
	})

	for _, tc := range []struct{ name, key string }{
		{"missing", ""},
		{"wrong", "not-the-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
			if tc.key != "" {
				req.Header.Set("X-API-Key", tc.key)
			}
			rec := httptest.NewRecorder()
			srv.authenticateAgent(srv.handleIngest)(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("ingest = %d, want 401", rec.Code)
			}
		})
	}
}

func TestAgentJoinRejectsBadToken(t *testing.T) {
	srv, _ := ingestServer(t)
	rec := postJSON(t, srv.handleAgentJoin, "/api/agents/join", joinRequest{
		Token: "never-issued", HostID: "remote-1",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("join with bad token = %d, want 401", rec.Code)
	}
}

func TestIngestAcceptsUnknownFieldsFromNewerAgent(t *testing.T) {
	srv, _ := ingestServer(t)

	// A field this server has never heard of must not sink the whole batch.
	payload := `{"agent_version":"9.9.9","future_field":{"nested":true},"samples":[
		{"host_id":"remote-1","hostname":"web-01","timestamp":"` +
		time.Now().Format(time.RFC3339) + `","cpu_usage":7}]}`

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	srv.handleIngest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ingest from newer agent = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestJoinCommandIsPasteable(t *testing.T) {
	got := joinCommand("https://sentinel.example.com", "abc123")
	want := "sys-sentient agent join --server https://sentinel.example.com --token abc123"
	if got != want {
		t.Errorf("joinCommand() = %q, want %q", got, want)
	}
}
