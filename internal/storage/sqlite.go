package storage

import (
	"database/sql"

	"sys-sentient/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := createTable(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func createTable(db *sql.DB) error {
	query := `
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
		top_processes TEXT
	);
	`
	_, err := db.Exec(query)
	return err
}

func (s *Store) Save(m *models.SystemState) error {
	query := `
	INSERT INTO metrics (
		timestamp, cpu_usage, memory_used, memory_total, 
		disk_read_bytes, disk_write_bytes, net_sent_bytes, net_recv_bytes, top_processes
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.Exec(query,
		m.Timestamp, m.CPUUsage, m.MemoryUsed, m.MemoryTotal,
		m.DiskReadBytes, m.DiskWriteBytes, m.NetSentBytes, m.NetRecvBytes, m.TopProcesses,
	)
	return err
}

func (s *Store) GetRecent(limit int) ([]models.SystemState, error) {
	query := `SELECT timestamp, cpu_usage, memory_used, memory_total, top_processes FROM metrics ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SystemState
	for rows.Next() {
		var m models.SystemState
		if err := rows.Scan(&m.Timestamp, &m.CPUUsage, &m.MemoryUsed, &m.MemoryTotal, &m.TopProcesses); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
