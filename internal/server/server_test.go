package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sys-sentient/internal/config"
	"testing"
)

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
