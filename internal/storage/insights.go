package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Insight is one stored AI analysis.
//
// The JSON tags are load-bearing. Without them this serialised as
// {"Content":…,"Timestamp":…} while the dashboard read content/timestamp, so
// every stored analysis was invisible and the console reported "No analysis
// yet" with a database full of them.
type Insight struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
	// HostID attributes the analysis to a machine. Without it a fleet cannot
	// tell whose problem an insight describes.
	HostID string `json:"host_id"`
	// Status is lifted out of the JSON body so a timeline can be filtered and
	// coloured without parsing every row.
	Status string `json:"status"`
}

// createInsightColumns widens the table for databases that predate these
// fields. New databases get them from the CREATE, which only runs once.
func createInsightColumns(db *sql.DB) error {
	for _, col := range []struct{ name, columnType string }{
		{"host_id", "TEXT NOT NULL DEFAULT ''"},
		{"status", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := addColumnIfMissing(db, "insights", col.name, col.columnType); err != nil {
			return err
		}
	}
	return nil
}

// SaveInsightRecord stores one analysis.
//
// Replaces a SaveInsight that took only the content string and bound
// CURRENT_TIMESTAMP, which gave second resolution — every other row type in
// this package stores milliseconds — and could not record which host the
// analysis was about.
func (s *Store) SaveInsightRecord(content, hostID string, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO insights (timestamp, content, host_id, status) VALUES (?, ?, ?, ?)`,
		sqlTime(at), content, hostID, extractInsightStatus(content))
	return err
}

// extractInsightStatus reads the model's verdict out of the stored JSON.
//
// Best effort: an unparseable body still stores, because losing the analysis
// over a missing field would be a worse outcome than an unlabelled row.
func extractInsightStatus(content string) string {
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return ""
	}
	return payload.Status
}

// InsightQuery filters a timeline read.
type InsightQuery struct {
	HostID string
	Status string
	// Range bounds the window. A zero To means "up to now".
	Range Range
	Limit int
}

// ListInsights returns stored analyses, newest first.
func (s *Store) ListInsights(q InsightQuery) ([]Insight, error) {
	query := `SELECT id, timestamp, content, COALESCE(host_id, ''), COALESCE(status, '')
	          FROM insights WHERE 1 = 1`
	args := make([]any, 0, 5)

	if q.HostID != "" {
		query += ` AND host_id = ?`
		args = append(args, q.HostID)
	}
	if q.Status != "" {
		query += ` AND status = ?`
		args = append(args, q.Status)
	}
	if !q.Range.From.IsZero() {
		query += ` AND timestamp >= ?`
		args = append(args, sqlTime(q.Range.From))
	}
	if !q.Range.To.IsZero() {
		query += ` AND timestamp <= ?`
		args = append(args, sqlTime(q.Range.To))
	}

	query += ` ORDER BY timestamp DESC, id DESC LIMIT ?`
	args = append(args, clampLimit(q.Limit))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list insights: %w", err)
	}
	defer func() { _ = rows.Close() }()

	insights := make([]Insight, 0, 16)
	for rows.Next() {
		var in Insight
		if err := rows.Scan(&in.ID, &in.Timestamp, &in.Content, &in.HostID, &in.Status); err != nil {
			return nil, err
		}
		insights = append(insights, in)
	}
	return insights, rows.Err()
}
