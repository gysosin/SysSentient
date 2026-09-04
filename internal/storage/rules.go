package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RuleOverride is an operator's change to a built-in alert rule.
//
// Only the differences are stored, not whole rules. The defaults are code, and
// an install that never touches them should follow the code as it improves
// rather than being pinned to whatever the defaults were on the day it was
// first started.
type RuleOverride struct {
	RuleID string `json:"rule_id"`
	// Threshold and For are nil when the operator has not changed them.
	Threshold *float64 `json:"threshold,omitempty"`
	ForSecs   *int     `json:"for_seconds,omitempty"`
	// Enabled is nil unless explicitly toggled.
	Enabled *bool `json:"enabled,omitempty"`
	// MutedUntil suppresses notifications without disabling evaluation, so
	// the alert still shows on the dashboard while it stops paging anyone.
	MutedUntil *time.Time `json:"muted_until,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
	UpdatedBy  string     `json:"updated_by"`
}

// Muted reports whether notifications are currently suppressed.
func (r RuleOverride) Muted(now time.Time) bool {
	return r.MutedUntil != nil && r.MutedUntil.After(now)
}

func createRuleTable(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS alert_rule_overrides (
		rule_id TEXT PRIMARY KEY,
		threshold REAL,
		for_seconds INTEGER,
		enabled INTEGER,
		muted_until DATETIME,
		updated_at DATETIME NOT NULL,
		updated_by TEXT NOT NULL DEFAULT ''
	);`)
	if err != nil {
		return fmt.Errorf("create alert_rule_overrides: %w", err)
	}
	return nil
}

// SaveRuleOverride records an operator's change.
func (s *Store) SaveRuleOverride(o RuleOverride) error {
	var mutedUntil any
	if o.MutedUntil != nil {
		mutedUntil = sqlTime(*o.MutedUntil)
	}
	_, err := s.db.Exec(`
		INSERT INTO alert_rule_overrides (rule_id, threshold, for_seconds, enabled, muted_until, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(rule_id) DO UPDATE SET
			threshold = excluded.threshold,
			for_seconds = excluded.for_seconds,
			enabled = excluded.enabled,
			muted_until = excluded.muted_until,
			updated_at = excluded.updated_at,
			updated_by = excluded.updated_by`,
		o.RuleID, o.Threshold, o.ForSecs, o.Enabled, mutedUntil, sqlTime(o.UpdatedAt), o.UpdatedBy)
	return err
}

// DeleteRuleOverride returns a rule to its built-in defaults.
func (s *Store) DeleteRuleOverride(ruleID string) error {
	res, err := s.db.Exec(`DELETE FROM alert_rule_overrides WHERE rule_id = ?`, ruleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRuleOverrides returns every stored change, keyed by rule id.
func (s *Store) ListRuleOverrides() (map[string]RuleOverride, error) {
	rows, err := s.db.Query(`
		SELECT rule_id, threshold, for_seconds, enabled, muted_until, updated_at, updated_by
		FROM alert_rule_overrides`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]RuleOverride, 8)
	for rows.Next() {
		var (
			o          RuleOverride
			threshold  sql.NullFloat64
			forSecs    sql.NullInt64
			enabled    sql.NullBool
			mutedUntil sql.NullTime
		)
		if err := rows.Scan(&o.RuleID, &threshold, &forSecs, &enabled, &mutedUntil,
			&o.UpdatedAt, &o.UpdatedBy); err != nil {
			return nil, err
		}
		if threshold.Valid {
			v := threshold.Float64
			o.Threshold = &v
		}
		if forSecs.Valid {
			v := int(forSecs.Int64)
			o.ForSecs = &v
		}
		if enabled.Valid {
			v := enabled.Bool
			o.Enabled = &v
		}
		if mutedUntil.Valid {
			v := mutedUntil.Time
			o.MutedUntil = &v
		}
		out[o.RuleID] = o
	}
	return out, rows.Err()
}

// ErrNoOverride reports that a rule is running on its defaults.
var ErrNoOverride = errors.New("rule has no stored override")
