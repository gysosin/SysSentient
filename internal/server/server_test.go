package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sys-sentient/internal/config"
	"sys-sentient/internal/storage"
	"testing"
	"time"
)

type fakeLogCollector struct {
	content string
	err     error
}

func (f fakeLogCollector) GetLogsWithTimeout(time.Duration) (string, error) {
	return f.content, f.err
}

func (f fakeLogCollector) GetLogsContextWithTimeout(context.Context, time.Duration) (string, error) {
	return f.content, f.err
}

func TestNewHTTPServerSetsProductionTimeouts(t *testing.T) {
	srv := newHTTPServer(":8080", http.NewServeMux())

	if srv.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout must be set to defend against slow headers")
	}
	if srv.ReadTimeout <= 0 {
		t.Fatal("ReadTimeout must be set to bound request reads")
	}
	if srv.WriteTimeout <= 0 {
		t.Fatal("WriteTimeout must be set to bound stuck responses")
	}
	if srv.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout must be set to bound idle keep-alive connections")
	}
}

func TestServerOriginPolicyAllowsConfiguredOrigins(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080", "http://localhost:3000"},
	}, nil, nil)

	if !srv.isOriginAllowed("http://localhost:3000") {
		t.Fatal("expected configured origin to be allowed")
	}
	if srv.isOriginAllowed("https://evil.example") {
		t.Fatal("expected unconfigured origin to be rejected")
	}
}

func TestServerOriginPolicyAllowsEmptyOrigin(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		AllowedOrigins: []string{"http://localhost:8080"},
	}, nil, nil)

	if !srv.isOriginAllowed("") {
		t.Fatal("expected empty origin to be allowed for same-origin and non-browser clients")
	}
}

func TestServerWebSocketAPIKeyUsesAuthValidator(t *testing.T) {
	srv := NewServer(config.ServerConfig{
		APIKey: "expected-key",
	}, nil, nil)

	if !srv.validWebSocketAPIKey("expected-key") {
		t.Fatal("expected matching WebSocket API key to be accepted")
	}
	if srv.validWebSocketAPIKey("expected-key-extra") {
		t.Fatal("expected similar WebSocket API key to be rejected")
	}
}

func TestHandleHealthReportsStorageStatus(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	srv := NewServer(config.ServerConfig{}, store, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected healthy status 200, got %d", rec.Code)
	}
	var healthy map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &healthy); err != nil {
		t.Fatalf("expected JSON health response: %v", err)
	}
	if healthy["database"] != "ok" {
		t.Fatalf("expected database status ok, got %q", healthy["database"])
	}

	if err := store.Close(); err != nil {
		t.Fatalf("failed to close test store: %v", err)
	}

	rec = httptest.NewRecorder()
	srv.handleHealth(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unhealthy status 503, got %d", rec.Code)
	}
}

func TestWriteInsightResponsePreservesJSONObject(t *testing.T) {
	rec := httptest.NewRecorder()
	raw := `{"status":"Healthy","summary":"ok","recommendedActions":[]}`

	if err := writeInsightResponse(rec, raw); err != nil {
		t.Fatalf("writeInsightResponse returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if payload["status"] != "Healthy" {
		t.Fatalf("expected status to be preserved, got %v", payload["status"])
	}
}

func TestWriteInsightResponseWrapsPlainText(t *testing.T) {
	rec := httptest.NewRecorder()

	if err := writeInsightResponse(rec, "plain diagnostic text"); err != nil {
		t.Fatalf("writeInsightResponse returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	if payload["detailedAnalysis"] != "plain diagnostic text" {
		t.Fatalf("expected plain text to be wrapped, got %v", payload["detailedAnalysis"])
	}
	if _, ok := payload["recommendedActions"].([]any); !ok {
		t.Fatalf("expected recommendedActions array, got %T", payload["recommendedActions"])
	}
}

func TestHandleLogsReturnsScrubbedContent(t *testing.T) {
	srv := NewServer(config.ServerConfig{}, nil, nil)
	srv.logReader = fakeLogCollector{
		content: "May 09 kernel warning for admin@example.com from 10.0.0.8 in /home/alice/app",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	srv.handleLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON logs response: %v", err)
	}

	content := payload["content"]
	for _, sensitive := range []string{"admin@example.com", "10.0.0.8", "/home/alice"} {
		if strings.Contains(content, sensitive) {
			t.Fatalf("expected log content to redact %q, got %q", sensitive, content)
		}
	}
	if !strings.Contains(content, "[EMAIL_REDACTED]") || !strings.Contains(content, "[IP_REDACTED]") {
		t.Fatalf("expected redaction markers in log content, got %q", content)
	}
}

func TestHandleLogsReportsCollectionFailure(t *testing.T) {
	srv := NewServer(config.ServerConfig{}, nil, nil)
	srv.logReader = fakeLogCollector{err: errors.New("journal unavailable")}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	srv.handleLogs(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON error response: %v", err)
	}
	if payload["error"] != "failed to collect logs" {
		t.Fatalf("expected sanitized log error, got %q", payload["error"])
	}
}
