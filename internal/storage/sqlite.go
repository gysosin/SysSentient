package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"sys-sentient/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	// Enable WAL mode for concurrency
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	if err := createTable(db); err != nil {
		db.Close()
		return nil, err
	}

	// Run migrations for new columns
	if err := migrateSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func createTable(db *sql.DB) error {
	var err error
	queryMetrics := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		cpu_usage REAL,
		memory_used INTEGER,
		memory_total INTEGER,
		disk_read_bytes INTEGER,
		disk_write_bytes INTEGER,
		net_sent_bytes INTEGER,
		net_recv_bytes INTEGER,
		temperature REAL,
		top_processes TEXT
	);
	`
	if _, err = db.Exec(queryMetrics); err != nil {
		return err
	}

	// Index for faster time-based queries
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);`); err != nil {
		return err
	}

	queryInsights := `
	CREATE TABLE IF NOT EXISTS insights (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		content TEXT
	);
	`
	_, err = db.Exec(queryInsights)
	return err
}

// migrateSchema adds new columns to existing tables
func migrateSchema(db *sql.DB) error {
	// List of new columns to add if they don't exist
	newColumns := []struct {
		name       string
		columnType string
	}{
		{"cpu_per_core", "TEXT"},
		{"swap_used", "INTEGER DEFAULT 0"},
		{"swap_total", "INTEGER DEFAULT 0"},
		{"disk_iops", "REAL DEFAULT 0"},
		{"load_avg_1", "REAL DEFAULT 0"},
		{"load_avg_5", "REAL DEFAULT 0"},
		{"load_avg_15", "REAL DEFAULT 0"},
		{"temperature", "REAL DEFAULT 0"},
	}

	for _, col := range newColumns {
		// SQLite doesn't have IF NOT EXISTS for ALTER TABLE, so we check first
		query := fmt.Sprintf("ALTER TABLE metrics ADD COLUMN %s %s", col.name, col.columnType)
		_, err := db.Exec(query)
		if err != nil {
			// Ignore "duplicate column" error
			if !strings.Contains(err.Error(), fmt.Sprintf("duplicate column name: %s", col.name)) {
				return fmt.Errorf("failed to add metrics.%s: %w", col.name, err)
			}
		}
	}
	return nil
}

func (s *Store) Save(m *models.SystemState) error {
	// Serialize cpu_per_core as JSON
	cpuPerCoreJSON, _ := json.Marshal(m.CPUPerCore)

	query := `
	INSERT INTO metrics (
		timestamp, cpu_usage, cpu_per_core, memory_used, memory_total,
		swap_used, swap_total, disk_read_bytes, disk_write_bytes, disk_iops,
		net_sent_bytes, net_recv_bytes, load_avg_1, load_avg_5, load_avg_15,
		temperature, top_processes
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query,
		m.Timestamp, m.CPUUsage, string(cpuPerCoreJSON), m.MemoryUsed, m.MemoryTotal,
		m.SwapUsed, m.SwapTotal, m.DiskReadBytes, m.DiskWriteBytes, m.DiskIOPS,
		m.NetSentBytes, m.NetRecvBytes, m.LoadAvg1, m.LoadAvg5, m.LoadAvg15,
		m.Temperature, m.TopProcesses,
	)
	return err
}

func (s *Store) PruneOldMetrics(hours int) error {
	query := `DELETE FROM metrics WHERE timestamp < datetime('now', ?)`
	// SQLite modifier: '-24 hours'
	modifier := fmt.Sprintf("-%d hours", hours)
	_, err := s.db.Exec(query, modifier)
	return err
}

func (s *Store) SaveInsight(content string) error {
	query := `INSERT INTO insights (timestamp, content) VALUES (CURRENT_TIMESTAMP, ?)`
	_, err := s.db.Exec(query, content)
	return err
}

func (s *Store) GetRecent(limit int) ([]models.SystemState, error) {
	query := `SELECT timestamp, cpu_usage, COALESCE(cpu_per_core, '[]'),
		memory_used, memory_total, COALESCE(swap_used, 0), COALESCE(swap_total, 0),
		disk_read_bytes, disk_write_bytes, COALESCE(disk_iops, 0),
		net_sent_bytes, net_recv_bytes, COALESCE(load_avg_1, 0), COALESCE(load_avg_5, 0), COALESCE(load_avg_15, 0),
		temperature, top_processes
		FROM metrics ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SystemState
	for rows.Next() {
		var m models.SystemState
		var cpuPerCoreJSON string
		if err := rows.Scan(
			&m.Timestamp, &m.CPUUsage, &cpuPerCoreJSON,
			&m.MemoryUsed, &m.MemoryTotal, &m.SwapUsed, &m.SwapTotal,
			&m.DiskReadBytes, &m.DiskWriteBytes, &m.DiskIOPS,
			&m.NetSentBytes, &m.NetRecvBytes, &m.LoadAvg1, &m.LoadAvg5, &m.LoadAvg15,
			&m.Temperature, &m.TopProcesses,
		); err != nil {
			return nil, err
		}
		// Deserialize cpu_per_core
		json.Unmarshal([]byte(cpuPerCoreJSON), &m.CPUPerCore)
		results = append(results, m)
	}
	return results, nil
}

type Insight struct {
	Timestamp string
	Content   string
}

func (s *Store) GetRecentInsights(limit int) ([]Insight, error) {
	query := `SELECT timestamp, content FROM insights ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Insight
	for rows.Next() {
		var i Insight
		if err := rows.Scan(&i.Timestamp, &i.Content); err != nil {
			return nil, err
		}
		results = append(results, i)
	}
	return results, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
