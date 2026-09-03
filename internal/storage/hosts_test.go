package storage

import (
	"path/filepath"
	"testing"
	"time"
)

// The hosts table was populated only by the agent ingest path, so an
// all-in-one install — the default — left it empty while metrics accumulated.
// Verified against a live database at the time: 41,553 metrics rows, zero
// hosts. The dashboard hid it behind `hosts.length || 1`.
func TestUpsertHostRegistersAndRefreshes(t *testing.T) {
	t.Parallel()
	store, err := NewStore(filepath.Join(t.TempDir(), "hosts.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertHost("host-a", "alpha", "1.0.0", first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("got %d hosts, want 1", len(hosts))
	}
	if hosts[0].HostID != "host-a" || hosts[0].Hostname != "alpha" {
		t.Fatalf("unexpected host: %+v", hosts[0])
	}

	// A second report from the same machine must refresh it, not duplicate it
	// — otherwise a host would appear once per poll forever.
	later := first.Add(time.Hour)
	if err := store.UpsertHost("host-a", "alpha-renamed", "1.1.0", later); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	hosts, err = store.ListHosts()
	if err != nil {
		t.Fatalf("list again: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("got %d hosts after re-reporting, want 1", len(hosts))
	}
	// A rename must not start a new history: that is the whole reason host id
	// is derived from machine-id rather than the hostname.
	if hosts[0].Hostname != "alpha-renamed" {
		t.Errorf("hostname not refreshed: %q", hosts[0].Hostname)
	}
	if hosts[0].AgentVersion != "1.1.0" {
		t.Errorf("agent version not refreshed: %q", hosts[0].AgentVersion)
	}
}
