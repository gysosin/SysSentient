package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func TestNewStore(t *testing.T) {
	// Use temp db
	dbPath := "test_metrics.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	if store.db == nil {
		t.Error("Database connection should not be nil")
	}
}

func TestNewStoreConfiguresSQLiteRuntime(t *testing.T) {
	dbPath := "test_runtime.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	if maxOpen := store.db.Stats().MaxOpenConnections; maxOpen != 1 {
		t.Fatalf("Expected one open SQLite connection, got %d", maxOpen)
	}

	var busyTimeout int
	if err := store.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("Failed to read busy_timeout pragma: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("Expected busy_timeout 5000, got %d", busyTimeout)
	}

	var foreignKeys int
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("Failed to read foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("Expected foreign key enforcement, got %d", foreignKeys)
	}
}

func TestSaveAndGetRecent(t *testing.T) {
	dbPath := "test_save.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Save a metric
	state := &models.SystemState{
		Timestamp:      time.Now(),
		CPUUsage:       50.5,
		CPUPerCore:     []float64{45.0, 55.0},
		MemoryUsed:     1024 * 1024 * 1024, // 1GB
		MemoryTotal:    4096 * 1024 * 1024, // 4GB
		SwapUsed:       0,
		SwapTotal:      2048 * 1024 * 1024,
		DiskReadBytes:  1000,
		DiskWriteBytes: 2000,
		DiskIOPS:       100.5,
		NetSentBytes:   5000,
		NetRecvBytes:   3000,
		LoadAvg1:       1.5,
		LoadAvg5:       1.2,
		LoadAvg15:      1.0,
		Temperature:    65.5,
		TopProcesses:   "chrome (5.0%, 256MB, alice)",
		Processes: []models.Process{
			{PID: 42, Name: "chrome", User: "alice", CPU: 5.0, Memory: 256, State: "Running"},
		},
	}

	err = store.Save(state)
	if err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Retrieve recent metrics
	states, err := store.GetRecent(1)
	if err != nil {
		t.Fatalf("Failed to get recent: %v", err)
	}

	if len(states) != 1 {
		t.Fatalf("Expected 1 state, got %d", len(states))
	}

	retrieved := states[0]
	if retrieved.CPUUsage != state.CPUUsage {
		t.Errorf("Expected CPU %f, got %f", state.CPUUsage, retrieved.CPUUsage)
	}

	if len(retrieved.CPUPerCore) != 2 {
		t.Errorf("Expected 2 CPU cores, got %d", len(retrieved.CPUPerCore))
	}
	if len(retrieved.Processes) != 1 {
		t.Fatalf("Expected 1 process, got %d", len(retrieved.Processes))
	}
	if retrieved.Processes[0].PID != 42 || retrieved.Processes[0].User != "alice" {
		t.Fatalf("Expected structured process to round trip, got %+v", retrieved.Processes[0])
	}
}

func TestSaveAndGetInsights(t *testing.T) {
	dbPath := "test_insights.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	insight := "System is running normally"
	err = store.SaveInsight(insight)
	if err != nil {
		t.Fatalf("Failed to save insight: %v", err)
	}

	insights, err := store.GetRecentInsights(1)
	if err != nil {
		t.Fatalf("Failed to get insights: %v", err)
	}

	if len(insights) != 1 {
		t.Fatalf("Expected 1 insight, got %d", len(insights))
	}

	if insights[0].Content != insight {
		t.Errorf("Expected insight %q, got %q", insight, insights[0].Content)
	}
}

func TestNewStoreMigratesExistingMetricsTable(t *testing.T) {
	dbPath := "test_migrate.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open setup db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME,
			cpu_usage REAL,
			memory_used INTEGER,
			memory_total INTEGER,
			disk_read_bytes INTEGER,
			disk_write_bytes INTEGER,
			net_sent_bytes INTEGER,
			net_recv_bytes INTEGER,
			top_processes TEXT
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create old schema: %v", err)
	}
	db.Close()

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to migrate store: %v", err)
	}
	defer store.Close()

	state := &models.SystemState{
		Timestamp:      time.Now(),
		CPUUsage:       10,
		CPUPerCore:     []float64{10},
		MemoryUsed:     1024,
		MemoryTotal:    4096,
		DiskReadBytes:  100,
		DiskWriteBytes: 200,
		NetSentBytes:   300,
		NetRecvBytes:   400,
		Temperature:    42.5,
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("Failed to save after migration: %v", err)
	}

	states, err := store.GetRecent(1)
	if err != nil {
		t.Fatalf("Failed to read after migration: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("Expected migrated metric, got %d", len(states))
	}
	if states[0].Temperature != state.Temperature {
		t.Fatalf("Expected temperature %.1f, got %.1f", state.Temperature, states[0].Temperature)
	}
	if states[0].Processes == nil {
		t.Fatal("Expected migrated metrics to return an empty process list")
	}
}

func TestMigrateSchemaIsIdempotent(t *testing.T) {
	dbPath := "test_migrate_idempotent.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	if err := migrateSchema(store.db); err != nil {
		t.Fatalf("Expected repeated migration to succeed: %v", err)
	}
}

func TestGetRecentDefaultsCorruptJSONColumnsToEmptySlices(t *testing.T) {
	dbPath := "test_corrupt_json.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.db.Exec(`
		INSERT INTO metrics (
			timestamp, cpu_usage, cpu_per_core, memory_used, memory_total,
			disk_read_bytes, disk_write_bytes, net_sent_bytes, net_recv_bytes,
			temperature, top_processes, processes
		) VALUES (CURRENT_TIMESTAMP, 1, 'not-json', 1, 1, 0, 0, 0, 0, 0, '', 'not-json')
	`)
	if err != nil {
		t.Fatalf("Failed to seed corrupt metric: %v", err)
	}

	states, err := store.GetRecent(1)
	if err != nil {
		t.Fatalf("Failed to read corrupt metric: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("Expected one metric, got %d", len(states))
	}
	if states[0].CPUPerCore == nil {
		t.Fatal("Expected corrupt cpu_per_core JSON to return an empty slice")
	}
	if states[0].Processes == nil {
		t.Fatal("Expected corrupt processes JSON to return an empty slice")
	}
}

func TestGetRecentRejectsNonPositiveLimit(t *testing.T) {
	dbPath := "test_limit.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Save(&models.SystemState{Timestamp: time.Now(), CPUUsage: 10, MemoryTotal: 1}); err != nil {
		t.Fatalf("Failed to save metric: %v", err)
	}

	states, err := store.GetRecent(-1)
	if err != nil {
		t.Fatalf("GetRecent returned error: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("Expected no metrics for negative limit, got %d", len(states))
	}

	insights, err := store.GetRecentInsights(0)
	if err != nil {
		t.Fatalf("GetRecentInsights returned error: %v", err)
	}
	if len(insights) != 0 {
		t.Fatalf("Expected no insights for zero limit, got %d", len(insights))
	}
}

func TestPruneOldMetrics(t *testing.T) {
	dbPath := "test_prune.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Save some test metrics
	for i := 0; i < 5; i++ {
		state := &models.SystemState{
			Timestamp:   time.Now(),
			CPUUsage:    float64(i * 10),
			MemoryUsed:  1024,
			MemoryTotal: 4096,
		}
		store.Save(state)
	}

	// Prune (this won't delete anything since they're all new)
	err = store.PruneOldMetrics(24)
	if err != nil {
		t.Fatalf("Failed to prune: %v", err)
	}

	// Verify metrics still exist
	states, _ := store.GetRecent(10)
	if len(states) != 5 {
		t.Errorf("Expected 5 metrics after pruning recent data, got %d", len(states))
	}
}

func TestPruneOldInsights(t *testing.T) {
	dbPath := "test_prune_insights.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + "-wal")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.db.Exec(`
		INSERT INTO insights (timestamp, content)
		VALUES (datetime('now', '-8 days'), 'old insight'), (CURRENT_TIMESTAMP, 'new insight')
	`)
	if err != nil {
		t.Fatalf("Failed to seed insights: %v", err)
	}

	if err := store.PruneOldInsights(7 * 24); err != nil {
		t.Fatalf("Failed to prune old insights: %v", err)
	}

	insights, err := store.GetRecentInsights(10)
	if err != nil {
		t.Fatalf("Failed to read insights: %v", err)
	}
	if len(insights) != 1 {
		t.Fatalf("Expected 1 insight after pruning, got %d", len(insights))
	}
	if insights[0].Content != "new insight" {
		t.Fatalf("Expected new insight to remain, got %q", insights[0].Content)
	}
}

func TestSaveAndReadUptime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "uptime.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	want := uint64(123456)
	if err := store.Save(&models.SystemState{
		Timestamp:     time.Now(),
		CPUUsage:      10,
		UptimeSeconds: want,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetRecent() returned %d rows, want 1", len(got))
	}
	if got[0].UptimeSeconds != want {
		t.Fatalf("UptimeSeconds = %d, want %d", got[0].UptimeSeconds, want)
	}
}

func TestListEndpointsReturnEmptySliceNotNil(t *testing.T) {
	// nil slices marshal to JSON `null`; clients then have to null-check a
	// list endpoint. Both list queries must return an allocated empty slice.
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	metrics, err := store.GetRecent(10)
	if err != nil {
		t.Fatalf("GetRecent() error = %v", err)
	}
	if metrics == nil {
		t.Fatal("GetRecent() returned nil, want an empty slice")
	}

	insights, err := store.GetRecentInsights(10)
	if err != nil {
		t.Fatalf("GetRecentInsights() error = %v", err)
	}
	if insights == nil {
		t.Fatal("GetRecentInsights() returned nil, want an empty slice")
	}
}

func TestSaveAndReadFilesystems(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fs.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	want := []models.Filesystem{
		{Mountpoint: "/", Device: "/dev/nvme0n1p2", FSType: "btrfs", TotalBytes: 500, UsedBytes: 400, FreeBytes: 100, UsedPercent: 80},
		{Mountpoint: "/boot", Device: "/dev/nvme0n1p1", FSType: "vfat", TotalBytes: 100, UsedBytes: 20, FreeBytes: 80, UsedPercent: 20},
	}

	if err := store.Save(&models.SystemState{Timestamp: time.Now(), Filesystems: want}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetRecent() returned %d rows, want 1", len(got))
	}
	if len(got[0].Filesystems) != 2 {
		t.Fatalf("round-tripped %d filesystems, want 2", len(got[0].Filesystems))
	}
	if got[0].Filesystems[0].Mountpoint != "/" || got[0].Filesystems[0].UsedPercent != 80 {
		t.Fatalf("filesystem[0] = %+v, want mountpoint / at 80%%", got[0].Filesystems[0])
	}
}

func TestFilesystemsDefaultToEmptySlice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fsnil.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	if err := store.Save(&models.SystemState{Timestamp: time.Now()}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent() error = %v", err)
	}
	if got[0].Filesystems == nil {
		t.Fatal("Filesystems decoded as nil, want an empty slice")
	}
}

func TestAlertEventRoundTripAndOrdering(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alerts.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	base := time.Now().UTC().Truncate(time.Second)
	events := []AlertEvent{
		{OccurredAt: base.Add(-2 * time.Hour), RuleID: "cpu", RuleName: "CPU", Metric: "cpu_usage", State: "firing", Severity: "warning", Value: 95, Threshold: 90, Hostname: "h1"},
		{OccurredAt: base.Add(-1 * time.Hour), RuleID: "cpu", RuleName: "CPU", Metric: "cpu_usage", State: "resolved", Severity: "warning", Value: 10, Threshold: 90, Hostname: "h1"},
		{OccurredAt: base, RuleID: "disk", RuleName: "Disk", Metric: "disk_percent", State: "firing", Severity: "critical", Value: 97, Threshold: 90, Hostname: "h1"},
	}
	for _, e := range events {
		if err := store.SaveAlertEvent(e); err != nil {
			t.Fatalf("SaveAlertEvent() error = %v", err)
		}
	}

	got, err := store.GetRecentAlertEvents(10)
	if err != nil {
		t.Fatalf("GetRecentAlertEvents() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	// Newest first.
	if got[0].RuleID != "disk" {
		t.Fatalf("first event = %q, want the newest (disk)", got[0].RuleID)
	}
	if got[0].Severity != "critical" || got[0].Value != 97 {
		t.Fatalf("event fields lost: %+v", got[0])
	}
}

func TestGetRecentAlertEventsReturnsEmptySlice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alerts-empty.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	got, err := store.GetRecentAlertEvents(10)
	if err != nil {
		t.Fatalf("GetRecentAlertEvents() error = %v", err)
	}
	if got == nil {
		t.Fatal("returned nil, want an empty slice (nil marshals to JSON null)")
	}
}

func TestPruneOldAlertEvents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alerts-prune.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	old := AlertEvent{OccurredAt: now.Add(-48 * time.Hour), RuleID: "old", RuleName: "Old", Metric: "cpu_usage", State: "firing", Severity: "warning", Value: 1, Threshold: 0, Hostname: "h"}
	recent := AlertEvent{OccurredAt: now.Add(-1 * time.Hour), RuleID: "new", RuleName: "New", Metric: "cpu_usage", State: "firing", Severity: "warning", Value: 1, Threshold: 0, Hostname: "h"}
	for _, e := range []AlertEvent{old, recent} {
		if err := store.SaveAlertEvent(e); err != nil {
			t.Fatalf("SaveAlertEvent() error = %v", err)
		}
	}

	if err := store.PruneOldAlertEvents(24); err != nil {
		t.Fatalf("PruneOldAlertEvents() error = %v", err)
	}

	got, err := store.GetRecentAlertEvents(10)
	if err != nil {
		t.Fatalf("GetRecentAlertEvents() error = %v", err)
	}
	if len(got) != 1 || got[0].RuleID != "new" {
		t.Fatalf("after prune got %+v, want only the recent event", got)
	}
}

func TestRetentionIsTimezoneIndependent(t *testing.T) {
	// go-sqlite3 binds time.Time using local wall time with a UTC offset
	// ("2026-09-02 01:44:32+05:30"), while PruneOldMetrics compares against
	// datetime('now', ...) which SQLite returns as UTC with no offset. The
	// comparison is lexicographic over two different representations, so the
	// effective retention window drifts by the host's UTC offset: east of
	// Greenwich rows linger past their window, west of it they are deleted
	// early.
	zones := []struct {
		name   string
		offset int // seconds east of UTC
	}{
		{name: "UTC", offset: 0},
		{name: "UTC+5:30 (Asia/Kolkata)", offset: 5*3600 + 30*60},
		{name: "UTC-8 (US Pacific)", offset: -8 * 3600},
		{name: "UTC+13 (Pacific/Apia)", offset: 13 * 3600},
	}

	for _, zone := range zones {
		t.Run(zone.name, func(t *testing.T) {
			loc := time.FixedZone(zone.name, zone.offset)

			dbPath := filepath.Join(t.TempDir(), "retention.db")
			store, err := NewStore(dbPath)
			if err != nil {
				t.Fatalf("NewStore() error = %v", err)
			}
			defer store.Close()

			now := time.Now()
			// One row safely inside a 24h window, one safely outside it.
			fresh := now.Add(-1 * time.Hour).In(loc)
			stale := now.Add(-30 * time.Hour).In(loc)

			for _, ts := range []time.Time{fresh, stale} {
				if err := store.Save(&models.SystemState{Timestamp: ts, CPUUsage: 1}); err != nil {
					t.Fatalf("Save() error = %v", err)
				}
			}

			if err := store.PruneOldMetrics(24); err != nil {
				t.Fatalf("PruneOldMetrics() error = %v", err)
			}

			remaining, err := store.GetRecent(10)
			if err != nil {
				t.Fatalf("GetRecent() error = %v", err)
			}

			if len(remaining) != 1 {
				t.Fatalf("after pruning a 1h-old and a 30h-old row with a 24h window, "+
					"%d rows remain, want exactly 1 (the fresh one)", len(remaining))
			}
			if age := time.Since(remaining[0].Timestamp); age > 2*time.Hour {
				t.Fatalf("the surviving row is %v old; the stale row was kept instead of the fresh one", age)
			}
		})
	}
}

func TestTimestampsRoundTripAcrossZones(t *testing.T) {
	// Ordering must be correct regardless of the zone a sample was recorded in.
	dbPath := filepath.Join(t.TempDir(), "ordering.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	base := time.Now().UTC().Truncate(time.Second)
	east := time.FixedZone("east", 13*3600)
	west := time.FixedZone("west", -8*3600)

	// Written oldest-first, but each in a different zone.
	samples := []struct {
		at  time.Time
		cpu float64
	}{
		{at: base.Add(-3 * time.Hour).In(east), cpu: 1},
		{at: base.Add(-2 * time.Hour).In(west), cpu: 2},
		{at: base.Add(-1 * time.Hour).In(time.UTC), cpu: 3},
	}
	for _, s := range samples {
		if err := store.Save(&models.SystemState{Timestamp: s.at, CPUUsage: s.cpu}); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	got, err := store.GetRecent(10)
	if err != nil {
		t.Fatalf("GetRecent() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	// GetRecent returns newest first.
	if got[0].CPUUsage != 3 || got[2].CPUUsage != 1 {
		t.Fatalf("rows out of order across zones: got cpu %v, %v, %v; want 3, 2, 1",
			got[0].CPUUsage, got[1].CPUUsage, got[2].CPUUsage)
	}
}

func TestMigrationNormalizesLegacyLocalTimestamps(t *testing.T) {
	// Databases written before the UTC fix contain local wall time with an
	// offset suffix. Opening the store must rewrite them, otherwise retention
	// and ordering stay wrong for the lifetime of the existing data.
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if err := store.Save(&models.SystemState{Timestamp: time.Now(), CPUUsage: 1}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Simulate the legacy encoding directly.
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	const legacy = "2026-09-02 01:44:32.730290991+05:30" // == 2026-09-01 20:14:32 UTC
	if _, err := raw.Exec(`UPDATE metrics SET timestamp = ?`, legacy); err != nil {
		t.Fatalf("seed legacy timestamp: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	// Reopening must migrate it.
	migrated, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() on legacy db error = %v", err)
	}
	defer migrated.Close()

	// Read the raw bytes: go-sqlite3 parses DATETIME columns into time.Time on
	// read, so scanning into a string would show the driver's formatting rather
	// than what is actually stored.
	var storedRaw string
	if err := migrated.db.QueryRow(`SELECT CAST(timestamp AS TEXT) FROM metrics LIMIT 1`).Scan(&storedRaw); err != nil {
		t.Fatalf("read migrated timestamp: %v", err)
	}
	if strings.Contains(storedRaw, "+05:30") {
		t.Fatalf("timestamp still carries a local offset after migration: %q", storedRaw)
	}

	// What matters is the instant, not the rendering.
	want := time.Date(2026, 9, 1, 20, 14, 32, 0, time.UTC)
	got, err := migrated.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetRecent() returned %d rows, want 1", len(got))
	}
	if !got[0].Timestamp.UTC().Equal(want) {
		t.Fatalf("migrated instant = %v, want %v", got[0].Timestamp.UTC(), want)
	}
	stored := storedRaw

	// Idempotent: a second open must not shift it again.
	if err := migrated.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	again, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("second NewStore() error = %v", err)
	}
	defer again.Close()

	var second string
	if err := again.db.QueryRow(`SELECT CAST(timestamp AS TEXT) FROM metrics LIMIT 1`).Scan(&second); err != nil {
		t.Fatalf("read twice-migrated timestamp: %v", err)
	}
	if second != stored {
		t.Fatalf("migration is not idempotent: %q then %q", stored, second)
	}
}

func TestHostInventory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hosts.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	base := time.Now().UTC().Truncate(time.Second)

	if err := store.UpsertHost("host-a", "web-01", "v0.2.0", base.Add(-time.Hour)); err != nil {
		t.Fatalf("UpsertHost() error = %v", err)
	}
	if err := store.UpsertHost("host-b", "db-01", "v0.2.0", base); err != nil {
		t.Fatalf("UpsertHost() error = %v", err)
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("ListHosts() = %d hosts, want 2", len(hosts))
	}
	// Most recently seen first.
	if hosts[0].HostID != "host-b" {
		t.Fatalf("first host = %q, want host-b (most recently seen)", hosts[0].HostID)
	}
}

func TestUpsertHostPreservesFirstSeenAndTracksRename(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "rename.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	first := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	later := time.Now().UTC().Truncate(time.Second)

	if err := store.UpsertHost("stable-id", "old-name", "v0.1.0", first); err != nil {
		t.Fatalf("UpsertHost() error = %v", err)
	}
	// Same machine, renamed and upgraded: identity must survive.
	if err := store.UpsertHost("stable-id", "new-name", "v0.2.0", later); err != nil {
		t.Fatalf("UpsertHost() error = %v", err)
	}

	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("a rename created a second host: %+v", hosts)
	}

	h := hosts[0]
	if h.Hostname != "new-name" {
		t.Errorf("hostname = %q, want new-name", h.Hostname)
	}
	if h.AgentVersion != "v0.2.0" {
		t.Errorf("agent version = %q, want v0.2.0", h.AgentVersion)
	}
	if !h.FirstSeen.UTC().Equal(first) {
		t.Errorf("first_seen = %v, want it preserved at %v", h.FirstSeen.UTC(), first)
	}
	if !h.LastSeen.UTC().Equal(later) {
		t.Errorf("last_seen = %v, want %v", h.LastSeen.UTC(), later)
	}
}

func TestUpsertHostIgnoresEmptyID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "emptyhost.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	if err := store.UpsertHost("", "nameless", "v0", time.Now()); err != nil {
		t.Fatalf("UpsertHost(\"\") error = %v, want a silent no-op", err)
	}
	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("an empty host id created an inventory row: %+v", hosts)
	}
}

func TestGetRecentForHostScopesResults(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "scoped.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	samples := []models.SystemState{
		{HostID: "a", Hostname: "web-01", Timestamp: now.Add(-3 * time.Minute), CPUUsage: 10},
		{HostID: "b", Hostname: "db-01", Timestamp: now.Add(-2 * time.Minute), CPUUsage: 20},
		{HostID: "a", Hostname: "web-01", Timestamp: now.Add(-1 * time.Minute), CPUUsage: 30},
	}
	for i := range samples {
		if err := store.Save(&samples[i]); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	hostA, err := store.GetRecentForHost("a", 10)
	if err != nil {
		t.Fatalf("GetRecentForHost() error = %v", err)
	}
	if len(hostA) != 2 {
		t.Fatalf("host a returned %d samples, want 2", len(hostA))
	}
	for _, sample := range hostA {
		if sample.HostID != "a" {
			t.Fatalf("host-scoped query leaked a sample from %q", sample.HostID)
		}
	}
	if hostA[0].CPUUsage != 30 {
		t.Fatalf("newest sample cpu = %v, want 30", hostA[0].CPUUsage)
	}

	// An empty id means "any host", preserving single-node behaviour.
	all, err := store.GetRecentForHost("", 10)
	if err != nil {
		t.Fatalf("GetRecentForHost(\"\") error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("unscoped query returned %d samples, want all 3", len(all))
	}

	// Unknown host is empty, not an error and not everything.
	none, err := store.GetRecentForHost("does-not-exist", 10)
	if err != nil {
		t.Fatalf("GetRecentForHost(unknown) error = %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown host returned %d samples, want 0", len(none))
	}
}

// Memory composition has to survive the round trip, not just the live socket.
//
// "90% memory used" means opposite things depending on how much of it is
// reclaimable page cache, so a dashboard that loses cached/buffers on reload
// reports a healthy host as one about to swap. Both columns arrive through
// migrateSchema, which makes them easy to add to the INSERT and forget in the
// SELECT — this pins both ends.
func TestSaveAndGetRecentPreservesMemoryComposition(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory_composition.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	const (
		wantCached  = uint64(9_663_676_416)
		wantBuffers = uint64(536_870_912)
	)

	if err := store.Save(&models.SystemState{
		HostID:        "host-a",
		Hostname:      "test-host",
		Timestamp:     time.Now().UTC(),
		MemoryUsed:    12_884_901_888,
		MemoryTotal:   34_359_738_368,
		MemoryCached:  wantCached,
		MemoryBuffers: wantBuffers,
	}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	recent, err := store.GetRecent(1)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 row, got %d", len(recent))
	}
	if recent[0].MemoryCached != wantCached {
		t.Errorf("GetRecent MemoryCached = %d, want %d", recent[0].MemoryCached, wantCached)
	}
	if recent[0].MemoryBuffers != wantBuffers {
		t.Errorf("GetRecent MemoryBuffers = %d, want %d", recent[0].MemoryBuffers, wantBuffers)
	}

	// The host-scoped query is a separate statement with its own column list.
	scoped, err := store.GetRecentForHost("host-a", 1)
	if err != nil {
		t.Fatalf("GetRecentForHost failed: %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("expected 1 host-scoped row, got %d", len(scoped))
	}
	if scoped[0].MemoryCached != wantCached || scoped[0].MemoryBuffers != wantBuffers {
		t.Errorf("GetRecentForHost lost memory composition: cached=%d buffers=%d",
			scoped[0].MemoryCached, scoped[0].MemoryBuffers)
	}
}
