package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sys-sentient/internal/models"
)

func TestBroadcastMetricsReportsFullQueue(t *testing.T) {
	hub := NewHub()
	for i := 0; i < cap(hub.broadcast); i++ {
		hub.broadcast <- []byte(`{"type":"metrics"}`)
	}

	err := hub.BroadcastMetrics(&models.SystemState{})
	if !errors.Is(err, ErrBroadcastQueueFull) {
		t.Fatalf("expected full queue error, got %v", err)
	}
}

func TestWebSocketUpgraderRejectsUnvalidatedCrossOriginRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws/metrics", nil)
	req.Header.Set("Origin", "https://evil.example")

	if upgrader.CheckOrigin(req) {
		t.Fatal("expected unvalidated cross-origin WebSocket upgrade to be rejected")
	}
}

func TestWebSocketUpgraderAllowsRouteValidatedCrossOriginRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws/metrics", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req = markWebSocketOriginValidated(req)

	if !upgrader.CheckOrigin(req) {
		t.Fatal("expected route-validated cross-origin WebSocket upgrade to be allowed")
	}
}
