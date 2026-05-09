package storage

import (
	"database/sql"
	"os"
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
