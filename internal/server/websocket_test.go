package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestHubRunExitsOnClose(t *testing.T) {
	// Hub.Run was an infinite for/select with no exit path, so it leaked on
	// every shutdown and connected clients were never sent a close frame.
	hub := NewHub()

	done := make(chan struct{})
	go func() {
		hub.Run()
		close(done)
	}()

	hub.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Hub.Run() did not return after Close()")
	}
}

func TestHubCloseIsIdempotent(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	hub.Close()
	// A second Close must not panic on a closed channel.
	hub.Close()
}

func TestHubCloseDisconnectsClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{hub: hub, send: make(chan []byte, 1)}
	hub.register <- client

	// Give the hub a moment to register before shutting down.
	deadline := time.Now().Add(time.Second)
	for hub.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.ClientCount() != 1 {
		t.Fatalf("ClientCount() = %d, want 1", hub.ClientCount())
	}

	hub.Close()

	// The client's send channel must be closed so its write pump terminates
	// rather than blocking forever.
	select {
	case _, open := <-client.send:
		if open {
			t.Fatal("client send channel still open after Close()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client send channel was not closed on shutdown")
	}
}
