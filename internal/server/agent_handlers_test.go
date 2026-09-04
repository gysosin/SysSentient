package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"path/filepath"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
	"sys-sentient/internal/models"
	"sys-sentient/internal/storage"
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
	want := "sys-sentient agent join --server https://sentinel.example.com --token abc123 --install-service"
	if got != want {
		t.Errorf("joinCommand() = %q, want %q", got, want)
	}
}

func TestIngestDoesNotAlertWhenAlertingIsDisabled(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "noalert.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// A daemon built with alerting off constructs no evaluator. The ingest
	// path had no enabled check of its own, so a fleet server used to alert
	// regardless of the setting.
	srv := NewServer(config.ServerConfig{}, testPrivacy(), store, nil, nil, nil)

	body, _ := json.Marshal(IngestRequest{
		Samples: []models.SystemState{{
			HostID: "h1", Hostname: "web-01", Timestamp: time.Now(),
			CPUUsage: 99, MemoryUsed: 1 << 30, MemoryTotal: 1 << 30,
		}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleIngest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ingest = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	events, err := store.GetRecentAlertEvents(10)
	if err != nil {
		t.Fatalf("GetRecentAlertEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("recorded %d alert events with alerting disabled, want 0", len(events))
	}
}

func TestBootstrapCommandsCoverAMachineWithNothingInstalled(t *testing.T) {
	cmds := bootstrapCommands("https://monitor.example.com", "tok123")

	// The Devices screen used to assume the binary was already there. These
	// must be runnable on a machine that has never heard of SysSentient.
	unix, ok := cmds["unix"]
	if !ok || !strings.Contains(unix, "/install.sh") || !strings.Contains(unix, "tok123") {
		t.Errorf("unix bootstrap = %q", unix)
	}
	win, ok := cmds["windows"]
	if !ok || !strings.Contains(win, "install.ps1") || !strings.Contains(win, "tok123") {
		t.Errorf("windows bootstrap = %q", win)
	}
	// The server's own address, so a copied command reaches the right host.
	if !strings.Contains(unix, "https://monitor.example.com") {
		t.Errorf("unix bootstrap does not name the server: %q", unix)
	}
}

func TestInstallScriptsAreServedAndRunnable(t *testing.T) {
	srv, _ := ingestServer(t)

	for path, marker := range map[string]string{
		"/install.sh":  "checksum mismatch",
		"/install.ps1": "checksum mismatch",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			srv.handleInstallScript(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d", path, rec.Code)
			}
			body := rec.Body.String()
			if len(body) < 500 {
				t.Fatalf("%s is %d bytes, implausibly short", path, len(body))
			}
			// The script is piped straight into a shell, often as root. It
			// must refuse an unverified binary rather than warn.
			if !strings.Contains(body, marker) {
				t.Errorf("%s does not refuse on a checksum mismatch", path)
			}
			// Not a download: a Content-Disposition would make a browser save
			// it instead of the shell executing it.
			if cd := rec.Header().Get("Content-Disposition"); cd != "" {
				t.Errorf("%s sets Content-Disposition %q", path, cd)
			}
			if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("%s is missing nosniff", path)
			}
		})
	}
}
