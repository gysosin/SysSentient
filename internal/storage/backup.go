package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// Backup writes a consistent copy of the database to destPath.
//
// Copying the file with cp while the daemon is running produces a corrupt
// backup: the database and its write-ahead log are two files that must agree,
// and a copy taken between them will not. That is the mistake people make by
// default, and it is silent — the corruption only surfaces on restore.
//
// SQLite's VACUUM INTO is the supported way to do this online. It takes a read
// lock, writes a fully checkpointed and defragmented copy, and leaves the
// running database untouched. The result needs no -wal or -shm alongside it.
func (s *Store) Backup(destPath string) error {
	if destPath == "" {
		return fmt.Errorf("backup destination is empty")
	}

	// VACUUM INTO refuses to overwrite, which is the safe default — but it
	// fails opaquely, so say what happened.
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("backup destination %q already exists", destPath)
	}

	if dir := filepath.Dir(destPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
	}

	// The path is an identifier here, not a bindable parameter.
	if _, err := s.db.Exec(`VACUUM INTO ?`, destPath); err != nil {
		return fmt.Errorf("backup to %q: %w", destPath, err)
	}

	// The database holds password hashes and session tokens. A backup that
	// lands world-readable is a credential leak, and the umask cannot be
	// relied on.
	if err := os.Chmod(destPath, 0o600); err != nil {
		return fmt.Errorf("restrict backup permissions: %w", err)
	}
	return nil
}

// IntegrityCheck reports whether the database is structurally sound.
//
// Worth running against a restored backup before trusting it: a corrupt file
// opens and answers simple queries perfectly well.
func (s *Store) IntegrityCheck() (string, error) {
	var result string
	if err := s.db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return "", fmt.Errorf("integrity check: %w", err)
	}
	return result, nil
}
