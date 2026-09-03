package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrTokenNotFound covers a token that never existed, was already redeemed
	// or has expired. Deliberately one error: telling a caller which of those
	// applies lets an attacker enumerate valid tokens.
	ErrTokenNotFound = errors.New("join token is not valid")
	// ErrAgentRevoked is returned when a known credential has been withdrawn.
	ErrAgentRevoked = errors.New("agent credential has been revoked")
)

// JoinToken is a single-use invitation for a machine to enrol.
//
// The whole fleet previously shared one static key, so there was no per-agent
// identity, no rotation and no revocation — withdrawing one machine's access
// meant changing the key on every machine at once.
type JoinToken struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedBy  string     `json:"created_by"`
	RedeemedAt *time.Time `json:"redeemed_at,omitempty"`
	RedeemedBy string     `json:"redeemed_by,omitempty"`
}

// Agent is an enrolled machine's credential.
type Agent struct {
	ID           string     `json:"id"`
	HostID       string     `json:"host_id"`
	Hostname     string     `json:"hostname"`
	Label        string     `json:"label"`
	CreatedAt    time.Time  `json:"created_at"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	AgentVersion string     `json:"agent_version"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

func createAgentTables(db *sql.DB) error {
	if _, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS join_tokens (
		id TEXT PRIMARY KEY,
		-- Only the hash is stored. A token readable from the database would
		-- let anyone with a copy of the file enrol a machine.
		token_hash TEXT NOT NULL UNIQUE,
		label TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL,
		created_by TEXT NOT NULL DEFAULT '',
		redeemed_at DATETIME,
		redeemed_by TEXT
	);`); err != nil {
		return fmt.Errorf("create join_tokens: %w", err)
	}

	if _, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		-- Same reasoning: the credential is only ever compared by hash.
		key_hash TEXT NOT NULL UNIQUE,
		host_id TEXT NOT NULL DEFAULT '',
		hostname TEXT NOT NULL DEFAULT '',
		label TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		last_seen_at DATETIME,
		agent_version TEXT NOT NULL DEFAULT '',
		revoked_at DATETIME
	);`); err != nil {
		return fmt.Errorf("create agents: %w", err)
	}

	// Every ingest request looks a credential up by hash, so this index is on
	// the hottest path in server mode.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_key ON agents(key_hash);`); err != nil {
		return fmt.Errorf("index agents: %w", err)
	}
	return nil
}

// CreateJoinToken stores the hash of a new invitation.
func (s *Store) CreateJoinToken(id, tokenHash, label, createdBy string, now, expiresAt time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO join_tokens (id, token_hash, label, created_at, expires_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, tokenHash, label, sqlTime(now), sqlTime(expiresAt), createdBy)
	return err
}

// RedeemJoinToken exchanges a valid token for an agent credential.
//
// The whole exchange is one transaction: a token consumed without an agent
// created would strand the machine holding it, and an agent created without
// consuming the token would make a single-use invitation reusable.
func (s *Store) RedeemJoinToken(tokenHash, agentID, keyHash, hostID, hostname, version string, now time.Time) (*Agent, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		tokenID    string
		label      string
		expiresAt  time.Time
		redeemedAt sql.NullTime
	)
	err = tx.QueryRow(`
		SELECT id, label, expires_at, redeemed_at FROM join_tokens WHERE token_hash = ?`,
		tokenHash).Scan(&tokenID, &label, &expiresAt, &redeemedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}

	// Already used, or expired. Both collapse to the same error so a caller
	// cannot distinguish "wrong token" from "token that existed".
	if redeemedAt.Valid || now.After(expiresAt) {
		return nil, ErrTokenNotFound
	}

	if _, err := tx.Exec(`
		UPDATE join_tokens SET redeemed_at = ?, redeemed_by = ? WHERE id = ?`,
		sqlTime(now), hostID, tokenID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`
		INSERT INTO agents (id, key_hash, host_id, hostname, label, created_at, agent_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		agentID, keyHash, hostID, hostname, label, sqlTime(now), version); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Agent{
		ID: agentID, HostID: hostID, Hostname: hostname, Label: label,
		CreatedAt: now, AgentVersion: version,
	}, nil
}

// AgentByKey looks up a credential for authenticating an ingest request.
func (s *Store) AgentByKey(keyHash string) (*Agent, error) {
	var a Agent
	var lastSeen, revoked sql.NullTime
	err := s.db.QueryRow(`
		SELECT id, host_id, hostname, label, created_at, last_seen_at, agent_version, revoked_at
		FROM agents WHERE key_hash = ?`, keyHash).
		Scan(&a.ID, &a.HostID, &a.Hostname, &a.Label, &a.CreatedAt, &lastSeen, &a.AgentVersion, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if revoked.Valid {
		// Distinguished from "unknown" here, unlike tokens: the caller holds a
		// credential that genuinely existed, and an agent that keeps retrying
		// forever because it cannot tell it was revoked is worse than one told
		// plainly to stop.
		a.RevokedAt = &revoked.Time
		return &a, ErrAgentRevoked
	}
	if lastSeen.Valid {
		a.LastSeenAt = &lastSeen.Time
	}
	return &a, nil
}

// TouchAgent records that a credential was used.
func (s *Store) TouchAgent(id, hostname, version string, now time.Time) error {
	_, err := s.db.Exec(`
		UPDATE agents SET last_seen_at = ?, hostname = ?, agent_version = ? WHERE id = ?`,
		sqlTime(now), hostname, version, id)
	return err
}

// RevokeAgent withdraws a credential.
//
// The row is kept rather than deleted, so the fleet list can show that a
// machine was removed and when — a silently vanishing host looks like a bug.
func (s *Store) RevokeAgent(id string, now time.Time) error {
	res, err := s.db.Exec(`UPDATE agents SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		sqlTime(now), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAgents returns every enrolled agent, newest first.
func (s *Store) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`
		SELECT id, host_id, hostname, label, created_at, last_seen_at, agent_version, revoked_at
		FROM agents ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	agents := make([]Agent, 0, 8)
	for rows.Next() {
		var a Agent
		var lastSeen, revoked sql.NullTime
		if err := rows.Scan(&a.ID, &a.HostID, &a.Hostname, &a.Label,
			&a.CreatedAt, &lastSeen, &a.AgentVersion, &revoked); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			a.LastSeenAt = &lastSeen.Time
		}
		if revoked.Valid {
			a.RevokedAt = &revoked.Time
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// ListJoinTokens returns tokens that are still usable, newest first.
func (s *Store) ListJoinTokens(now time.Time) ([]JoinToken, error) {
	rows, err := s.db.Query(`
		SELECT id, label, created_at, expires_at, created_by, redeemed_at, redeemed_by
		FROM join_tokens
		WHERE redeemed_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC`, sqlTime(now))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	tokens := make([]JoinToken, 0, 4)
	for rows.Next() {
		var t JoinToken
		var redeemedAt sql.NullTime
		var redeemedBy sql.NullString
		if err := rows.Scan(&t.ID, &t.Label, &t.CreatedAt, &t.ExpiresAt,
			&t.CreatedBy, &redeemedAt, &redeemedBy); err != nil {
			return nil, err
		}
		if redeemedAt.Valid {
			t.RedeemedAt = &redeemedAt.Time
		}
		t.RedeemedBy = redeemedBy.String
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// PruneExpiredJoinTokens removes invitations nobody used.
func (s *Store) PruneExpiredJoinTokens(now time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM join_tokens WHERE redeemed_at IS NULL AND expires_at < ?`,
		sqlTime(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
