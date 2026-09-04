package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newAgentStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// mintToken creates an invitation and returns its hash, ready to redeem.
func mintToken(t *testing.T, s *Store, label string, ttl time.Duration) string {
	t.Helper()
	now := time.Now()
	hash := "hash-" + label
	if err := s.CreateJoinToken("tok-"+label, hash, label, "admin@example.com", now, now.Add(ttl)); err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	return hash
}

func TestRedeemJoinTokenCreatesAgent(t *testing.T) {
	s := newAgentStore(t)
	hash := mintToken(t, s, "web-01", time.Hour)

	agent, err := s.RedeemJoinToken(hash, "agent-1", "key-1", "host-1", "web-01.example.com", "0.1.0", time.Now())
	if err != nil {
		t.Fatalf("RedeemJoinToken: %v", err)
	}
	if agent.HostID != "host-1" {
		t.Errorf("HostID = %q, want host-1", agent.HostID)
	}
	// The label follows the invitation onto the agent, so an operator sees the
	// name they chose rather than a bare hostname.
	if agent.Label != "web-01" {
		t.Errorf("Label = %q, want web-01", agent.Label)
	}

	got, err := s.AgentByKey("key-1")
	if err != nil {
		t.Fatalf("AgentByKey: %v", err)
	}
	if got.ID != "agent-1" {
		t.Errorf("ID = %q, want agent-1", got.ID)
	}
}

func TestRedeemJoinTokenIsSingleUse(t *testing.T) {
	s := newAgentStore(t)
	hash := mintToken(t, s, "web-01", time.Hour)

	if _, err := s.RedeemJoinToken(hash, "agent-1", "key-1", "host-1", "h1", "0.1.0", time.Now()); err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	// A replayed invitation must not mint a second credential: an attacker who
	// captures a token in transit would otherwise get a permanent foothold.
	_, err := s.RedeemJoinToken(hash, "agent-2", "key-2", "host-2", "h2", "0.1.0", time.Now())
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("replay err = %v, want ErrTokenNotFound", err)
	}

	if _, err := s.AgentByKey("key-2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("replay created a credential: %v", err)
	}
}

func TestRedeemJoinTokenRejectsExpired(t *testing.T) {
	s := newAgentStore(t)
	hash := mintToken(t, s, "stale", -time.Minute)

	_, err := s.RedeemJoinToken(hash, "agent-1", "key-1", "host-1", "h1", "0.1.0", time.Now())
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("expired err = %v, want ErrTokenNotFound", err)
	}
}

func TestRedeemJoinTokenRejectsUnknown(t *testing.T) {
	s := newAgentStore(t)
	_, err := s.RedeemJoinToken("never-issued", "agent-1", "key-1", "host-1", "h1", "0.1.0", time.Now())
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("unknown err = %v, want ErrTokenNotFound", err)
	}
}

func TestRevokedAgentIsRejectedButStillListed(t *testing.T) {
	s := newAgentStore(t)
	hash := mintToken(t, s, "web-01", time.Hour)
	if _, err := s.RedeemJoinToken(hash, "agent-1", "key-1", "host-1", "h1", "0.1.0", time.Now()); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := s.RevokeAgent("agent-1", time.Now()); err != nil {
		t.Fatalf("RevokeAgent: %v", err)
	}

	_, err := s.AgentByKey("key-1")
	if !errors.Is(err, ErrAgentRevoked) {
		t.Fatalf("after revoke err = %v, want ErrAgentRevoked", err)
	}

	// The row survives so the fleet list can explain the machine's absence.
	agents, err := s.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len(agents) = %d, want 1", len(agents))
	}
	if agents[0].RevokedAt == nil {
		t.Error("RevokedAt is nil on a revoked agent")
	}
}

func TestRevokeAgentTwiceReportsNotFound(t *testing.T) {
	s := newAgentStore(t)
	hash := mintToken(t, s, "web-01", time.Hour)
	if _, err := s.RedeemJoinToken(hash, "agent-1", "key-1", "host-1", "h1", "0.1.0", time.Now()); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := s.RevokeAgent("agent-1", time.Now()); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	if err := s.RevokeAgent("agent-1", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("second revoke err = %v, want ErrNotFound", err)
	}
}

func TestTouchAgentRecordsLastSeen(t *testing.T) {
	s := newAgentStore(t)
	hash := mintToken(t, s, "web-01", time.Hour)
	if _, err := s.RedeemJoinToken(hash, "agent-1", "key-1", "host-1", "h1", "0.1.0", time.Now()); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	now := time.Now().Truncate(time.Millisecond)
	if err := s.TouchAgent("agent-1", "renamed.example.com", "0.2.0", now); err != nil {
		t.Fatalf("TouchAgent: %v", err)
	}

	got, err := s.AgentByKey("key-1")
	if err != nil {
		t.Fatalf("AgentByKey: %v", err)
	}
	if got.LastSeenAt == nil {
		t.Fatal("LastSeenAt is nil after TouchAgent")
	}
	// Guards the timestamp-format bug that silently nulled columns once before:
	// a round-tripped time must come back as the same instant.
	if diff := got.LastSeenAt.Sub(now); diff > time.Second || diff < -time.Second {
		t.Errorf("LastSeenAt = %v, want ~%v", got.LastSeenAt, now)
	}
	if got.AgentVersion != "0.2.0" {
		t.Errorf("AgentVersion = %q, want 0.2.0", got.AgentVersion)
	}
}

func TestListJoinTokensHidesUsedAndExpired(t *testing.T) {
	s := newAgentStore(t)
	live := mintToken(t, s, "live", time.Hour)
	mintToken(t, s, "expired", -time.Minute)
	used := mintToken(t, s, "used", time.Hour)
	if _, err := s.RedeemJoinToken(used, "agent-1", "key-1", "host-1", "h1", "0.1.0", time.Now()); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	_ = live

	tokens, err := s.ListJoinTokens(time.Now())
	if err != nil {
		t.Fatalf("ListJoinTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("len(tokens) = %d, want 1 (only the live one)", len(tokens))
	}
	if tokens[0].Label != "live" {
		t.Errorf("Label = %q, want live", tokens[0].Label)
	}
}

func TestPruneExpiredJoinTokens(t *testing.T) {
	s := newAgentStore(t)
	mintToken(t, s, "live", time.Hour)
	mintToken(t, s, "expired", -time.Minute)

	n, err := s.PruneExpiredJoinTokens(time.Now())
	if err != nil {
		t.Fatalf("PruneExpiredJoinTokens: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}
}

func TestAlertEventsCarryHostID(t *testing.T) {
	s := newAgentStore(t)
	now := time.Now()

	// Two machines that share a hostname — the case that made alerts render as
	// indistinguishable duplicates.
	for _, hostID := range []string{"host-a", "host-b"} {
		if err := s.SaveAlertEvent(AlertEvent{
			OccurredAt: now, RuleID: "cpu-high", RuleName: "CPU sustained high",
			Metric: "cpu_usage", State: "firing", Severity: "warning",
			Value: 95, Threshold: 90, Hostname: "fedora", HostID: hostID,
		}); err != nil {
			t.Fatalf("SaveAlertEvent(%s): %v", hostID, err)
		}
	}

	events, err := s.GetRecentAlertEvents(10)
	if err != nil {
		t.Fatalf("GetRecentAlertEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}

	seen := map[string]bool{}
	for _, e := range events {
		if e.HostID == "" {
			t.Errorf("event for %q has no host id", e.Hostname)
		}
		seen[e.HostID] = true
	}
	if !seen["host-a"] || !seen["host-b"] {
		t.Errorf("host ids not distinguishable: %v", seen)
	}
}

func TestAlertEventsMigrateAnOlderDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// A database written before host_id existed.
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := legacy.Exec(`
		CREATE TABLE alert_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			occurred_at DATETIME NOT NULL,
			rule_id TEXT NOT NULL, rule_name TEXT NOT NULL, metric TEXT NOT NULL,
			state TEXT NOT NULL, severity TEXT NOT NULL,
			value REAL NOT NULL, threshold REAL NOT NULL,
			hostname TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO alert_events (occurred_at, rule_id, rule_name, metric, state, severity, value, threshold, hostname)
		VALUES ('2026-09-01 10:00:00.000','cpu-high','CPU','cpu_usage','firing','warning',95,90,'old-host');`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore on a legacy database: %v", err)
	}
	defer func() { _ = store.Close() }()

	events, err := store.GetRecentAlertEvents(10)
	if err != nil {
		t.Fatalf("GetRecentAlertEvents: %v", err)
	}
	// The pre-existing row must survive the migration, with an empty host id
	// rather than a failed read.
	if len(events) != 1 {
		t.Fatalf("got %d events after migration, want 1", len(events))
	}
	if events[0].Hostname != "old-host" || events[0].HostID != "" {
		t.Errorf("migrated row = %+v, want hostname old-host and an empty host id", events[0])
	}
}
