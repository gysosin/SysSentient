package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sample(hostID string, cpu float64) models.SystemState {
	return models.SystemState{HostID: hostID, Hostname: hostID, Timestamp: time.Now(), CPUUsage: cpu}
}

func TestSpoolRoundTripAndCommit(t *testing.T) {
	spool, err := NewSpool(filepath.Join(t.TempDir(), "spool.json"), 100)
	if err != nil {
		t.Fatalf("NewSpool() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := spool.Append(sample("h", float64(i))); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	if n, _ := spool.Len(); n != 5 {
		t.Fatalf("Len() = %d, want 5", n)
	}

	batch, err := spool.Peek(3)
	if err != nil {
		t.Fatalf("Peek() error = %v", err)
	}
	if len(batch) != 3 || batch[0].CPUUsage != 0 {
		t.Fatalf("Peek(3) = %d samples starting at cpu %v, want 3 starting at 0 (oldest first)", len(batch), batch[0].CPUUsage)
	}

	// Peek must not remove: a failed send has to be retryable.
	if n, _ := spool.Len(); n != 5 {
		t.Fatalf("Peek removed samples: Len() = %d, want 5", n)
	}

	if err := spool.Commit(3); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	remaining, _ := spool.Peek(10)
	if len(remaining) != 2 || remaining[0].CPUUsage != 3 {
		t.Fatalf("after Commit(3), remaining = %d starting at cpu %v, want 2 starting at 3", len(remaining), remaining[0].CPUUsage)
	}
}

func TestSpoolIsBounded(t *testing.T) {
	// An agent offline for a week must not fill the disk it is monitoring.
	spool, err := NewSpool(filepath.Join(t.TempDir(), "bounded.json"), 10)
	if err != nil {
		t.Fatalf("NewSpool() error = %v", err)
	}

	for i := 0; i < 50; i++ {
		if err := spool.Append(sample("h", float64(i))); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	n, _ := spool.Len()
	if n != 10 {
		t.Fatalf("Len() = %d, want the capacity of 10", n)
	}
	// The newest samples are the ones kept.
	batch, _ := spool.Peek(10)
	if batch[len(batch)-1].CPUUsage != 49 {
		t.Fatalf("newest retained sample cpu = %v, want 49", batch[len(batch)-1].CPUUsage)
	}
}

func TestSpoolSurvivesCorruption(t *testing.T) {
	// A truncated spool must not wedge the agent permanently.
	path := filepath.Join(t.TempDir(), "corrupt.json")
	spool, err := NewSpool(path, 10)
	if err != nil {
		t.Fatalf("NewSpool() error = %v", err)
	}
	if err := spool.Append(sample("h", 1)); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if err := writeFile(path, "{not json"); err != nil {
		t.Fatalf("corrupt spool: %v", err)
	}

	if n, err := spool.Len(); err != nil || n != 0 {
		t.Fatalf("Len() = %d, %v; want 0 and no error on a corrupt spool", n, err)
	}
	if err := spool.Append(sample("h", 2)); err != nil {
		t.Fatalf("Append() after corruption error = %v", err)
	}
	if n, _ := spool.Len(); n != 1 {
		t.Fatalf("Len() = %d after recovery, want 1", n)
	}
}

func TestFlushDeliversAndDrains(t *testing.T) {
	var (
		mu       sync.Mutex
		received int
		gotKey   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Samples []models.SystemState `json:"samples"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		received += len(req.Samples)
		gotKey = r.Header.Get("X-API-Key")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]int{"accepted": len(req.Samples), "rejected": 0})
	}))
	defer srv.Close()

	client, err := New(Options{
		ServerURL: srv.URL, Key: "agent-secret", AgentVersion: "v9",
		SpoolPath: filepath.Join(t.TempDir(), "s.json"), BatchSize: 10, Logger: quietLogger(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := client.Enqueue(sample("h", float64(i))); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if received != 3 {
		t.Fatalf("server received %d samples, want 3", received)
	}
	if gotKey != "agent-secret" {
		t.Fatalf("agent key header = %q, want agent-secret", gotKey)
	}
	if client.Pending() != 0 {
		t.Fatalf("Pending() = %d after a successful flush, want 0", client.Pending())
	}
}

func TestFlushRetainsSamplesWhenTheServerIsUnreachable(t *testing.T) {
	// The whole point of the spool: a network partition must lose nothing.
	client, err := New(Options{
		ServerURL: "http://127.0.0.1:1", // nothing listening
		Key:       "k",
		SpoolPath: filepath.Join(t.TempDir(), "outage.json"),
		BatchSize: 10, Logger: quietLogger(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for i := 0; i < 4; i++ {
		if err := client.Enqueue(sample("h", float64(i))); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	if err := client.Flush(context.Background()); err == nil {
		t.Fatal("Flush() = nil against an unreachable server, want an error")
	}
	if client.Pending() != 4 {
		t.Fatalf("Pending() = %d after a failed flush, want all 4 retained", client.Pending())
	}
}

func TestFlushSurfacesAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := New(Options{
		ServerURL: srv.URL, Key: "wrong",
		SpoolPath: filepath.Join(t.TempDir(), "auth.json"),
		BatchSize: 10, Logger: quietLogger(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = client.Enqueue(sample("h", 1))

	err = client.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() = nil on a 401, want an error")
	}
	// Samples must survive a rejected key so a fixed config recovers them.
	if client.Pending() != 1 {
		t.Fatalf("Pending() = %d after auth failure, want 1", client.Pending())
	}
}

func TestNewRequiresServerURL(t *testing.T) {
	if _, err := New(Options{SpoolPath: filepath.Join(t.TempDir(), "x.json")}); err == nil {
		t.Fatal("New() = nil error with no server URL")
	}
}

func writeFile(path, contents string) error {
	return osWriteFile(path, []byte(contents))
}
