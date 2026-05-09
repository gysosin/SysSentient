package server

import (
	"errors"
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
