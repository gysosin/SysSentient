package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

// Every timestamp must be stored in a form SQLite's own date functions can
// read. Binding a raw time.Time does not satisfy that: database/sql leaves the
// encoding to the driver, and modernc.org/sqlite writes Go's String() form,
// which datetime() cannot parse.
//
// The consequences were not theoretical. The normalisation migration NULLed
// 158 rows of a nullable column and aborted the daemon's start-up on a NOT
// NULL one.
func TestStoredTimestampsAreParseableBySQLite(t *testing.T) {
	t.Parallel()
	store, err := NewStore(filepath.Join(t.TempDir(), "ts.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	when := time.Date(2026, 9, 3, 17, 52, 20, 885000000, time.UTC)

	if err := store.Save(&models.SystemState{
		HostID: "h1", Hostname: "host", Timestamp: when, MemoryTotal: 1,
	}); err != nil {
		t.Fatalf("save metric: %v", err)
	}
	if err := store.SaveAlertEvent(AlertEvent{
		OccurredAt: when, RuleID: "r1", RuleName: "rule", Metric: "cpu",
		State: "firing", Severity: "warning", Hostname: "host",
	}); err != nil {
		t.Fatalf("save alert event: %v", err)
	}

	for _, tc := range []struct{ table, column string }{
		{"metrics", "timestamp"},
		{"alert_events", "occurred_at"},
	} {
		var unparseable int
		q := "SELECT count(*) FROM " + tc.table + " WHERE datetime(" + tc.column + ") IS NULL"
		if err := store.db.QueryRow(q).Scan(&unparseable); err != nil {
			t.Fatalf("%s: %v", tc.table, err)
		}
		if unparseable != 0 {
			var sample string
			_ = store.db.QueryRow("SELECT quote(" + tc.column + ") FROM " + tc.table).Scan(&sample)
			t.Errorf("%s.%s is not parseable by SQLite: %s", tc.table, tc.column, sample)
		}
	}
}

// Timestamps must also survive the round trip, or fixing the storage format
// would just move the bug to the read path.
func TestTimestampsRoundTrip(t *testing.T) {
	t.Parallel()
	store, err := NewStore(filepath.Join(t.TempDir(), "rt.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	when := time.Date(2026, 9, 3, 17, 52, 20, 885000000, time.UTC)
	if err := store.Save(&models.SystemState{
		HostID: "h1", Hostname: "host", Timestamp: when, MemoryTotal: 1,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.GetRecent(1)
	if err != nil || len(got) != 1 {
		t.Fatalf("read back: %v (%d rows)", err, len(got))
	}
	// Millisecond precision is what the stored layout keeps.
	if d := got[0].Timestamp.UTC().Sub(when); d > time.Millisecond || d < -time.Millisecond {
		t.Errorf("timestamp drifted by %v: stored %s, read %s",
			d, when.Format(time.RFC3339Nano), got[0].Timestamp.UTC().Format(time.RFC3339Nano))
	}
}

// The migration must never destroy a value it cannot parse, and must never
// abort start-up. This reproduces both failures directly: a Go time.String()
// value in a nullable column and in a NOT NULL one.
func TestMigrationLeavesUnparseableTimestampsIntact(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	// Build the schema, then plant the exact value that broke production.
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	const goFormat = "2026-09-03 17:20:12.19135217 +0000 UTC"
	if _, err := store.db.Exec(
		`INSERT INTO alert_events (occurred_at, rule_id, rule_name, metric, state, severity, value, threshold, hostname)
		 VALUES (?, 'r', 'rule', 'cpu', 'firing', 'warning', 1, 2, 'h')`, goFormat); err != nil {
		t.Fatalf("plant alert event: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO metrics (timestamp, cpu_usage, memory_used, memory_total) VALUES (?, 1, 1, 1)`,
		goFormat); err != nil {
		t.Fatalf("plant metric: %v", err)
	}
	_ = store.Close()

	// Reopening runs the migration. It previously aborted here.
	reopened, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("migration aborted start-up: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	var nulled int
	if err := reopened.db.QueryRow(
		`SELECT (SELECT count(*) FROM metrics WHERE timestamp IS NULL)
		      + (SELECT count(*) FROM alert_events WHERE occurred_at IS NULL)`).Scan(&nulled); err != nil {
		t.Fatalf("count: %v", err)
	}
	if nulled != 0 {
		t.Errorf("migration destroyed %d timestamp(s) it could not parse", nulled)
	}

	// And having repaired them, they should now be usable.
	var stillUnparseable int
	_ = reopened.db.QueryRow(`SELECT count(*) FROM alert_events WHERE datetime(occurred_at) IS NULL`).Scan(&stillUnparseable)
	if stillUnparseable != 0 {
		var sample sql.NullString
		_ = reopened.db.QueryRow(`SELECT quote(occurred_at) FROM alert_events`).Scan(&sample)
		t.Errorf("migration left %d unparseable value(s), e.g. %s", stillUnparseable, sample.String)
	}
}
