package storage

import (
	"database/sql"
	"errors"
	"time"
)

// SessionRecord stores the SHA-256 of a session token, never the token.
type SessionRecord struct {
	TokenHash  string
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

func (s *Store) CreateSession(rec SessionRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`,
		rec.TokenHash, rec.UserID, sqlTime(rec.CreatedAt), sqlTime(rec.ExpiresAt), sqlTime(rec.LastSeenAt))
	return err
}

func (s *Store) GetSession(tokenHash string) (*SessionRecord, error) {
	var rec SessionRecord
	err := s.db.QueryRow(`
		SELECT token_hash, user_id, created_at, expires_at, last_seen_at
		FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&rec.TokenHash, &rec.UserID, &rec.CreatedAt, &rec.ExpiresAt, &rec.LastSeenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rec.CreatedAt, rec.ExpiresAt, rec.LastSeenAt = rec.CreatedAt.UTC(), rec.ExpiresAt.UTC(), rec.LastSeenAt.UTC()
	return &rec, nil
}

// TouchSession records activity and slides the idle expiry forward.
func (s *Store) TouchSession(tokenHash string, lastSeen, expiresAt time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`,
		sqlTime(lastSeen), sqlTime(expiresAt), tokenHash)
	return err
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteUserSessions revokes every session for a user except keepTokenHash —
// used after a password change so the changing browser stays signed in while
// every other device is signed out.
func (s *Store) DeleteUserSessions(userID, keepTokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, keepTokenHash)
	return err
}

func (s *Store) PruneExpiredSessions(now time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, sqlTime(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
