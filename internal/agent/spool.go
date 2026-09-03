// Package agent implements push-mode collection: sample locally, buffer to
// disk, and forward to a sys-sentient server.
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"sys-sentient/internal/models"
)

// Spool is a bounded, durable buffer of samples awaiting delivery.
//
// Without it, a network partition silently loses every sample taken during the
// outage — which is exactly the window an operator most wants to inspect
// afterwards. Samples are appended as JSON lines and removed only once the
// server has acknowledged them.
//
// Deliberately a plain file rather than SQLite: the agent should not need cgo
// or a database, and append-plus-rewrite is sufficient for a bounded buffer.
type Spool struct {
	mu       sync.Mutex
	path     string
	capacity int
}

// NewSpool opens (or creates) a spool file. capacity bounds how many samples
// are retained; beyond it the oldest are dropped, because an agent that has
// been offline for a week must not fill the disk it is monitoring.
func NewSpool(path string, capacity int) (*Spool, error) {
	if capacity < 1 {
		capacity = 5000
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		// 0750: the spool holds this host's metrics; other users have no
		// reason to read them.
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create spool directory: %w", err)
		}
	}
	return &Spool{path: path, capacity: capacity}, nil
}

// Append adds a sample to the buffer.
func (s *Spool) Append(sample models.SystemState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	samples, err := s.readLocked()
	if err != nil {
		return err
	}

	samples = append(samples, sample)
	if len(samples) > s.capacity {
		// Drop oldest: recent data is more useful than a complete history of
		// a long outage, and the buffer must stay bounded.
		samples = samples[len(samples)-s.capacity:]
	}

	return s.writeLocked(samples)
}

// Peek returns up to n of the oldest buffered samples without removing them.
// They are removed by Commit once the server has acknowledged them, so a failed
// send never loses data.
func (s *Spool) Peek(n int) ([]models.SystemState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	samples, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	if n > 0 && len(samples) > n {
		samples = samples[:n]
	}
	return samples, nil
}

// Commit removes the oldest n samples after a successful send.
func (s *Spool) Commit(n int) error {
	if n <= 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	samples, err := s.readLocked()
	if err != nil {
		return err
	}
	if n >= len(samples) {
		return s.writeLocked(nil)
	}
	return s.writeLocked(samples[n:])
}

// Len reports how many samples are buffered.
func (s *Spool) Len() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	samples, err := s.readLocked()
	if err != nil {
		return 0, err
	}
	return len(samples), nil
}

func (s *Spool) readLocked() ([]models.SystemState, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read spool: %w", err)
	}

	if len(raw) == 0 {
		return nil, nil
	}

	return decodeSpool(raw), nil
}

// decodeSpool parses a spool file, treating an unparseable one as empty.
//
// This is deliberate recovery rather than a swallowed error: a truncated or
// corrupt spool (killed mid-write, disk full) must not wedge the agent
// forever. Losing the buffer is bad; refusing to ever collect again is worse.
// The next write replaces the file.
func decodeSpool(raw []byte) []models.SystemState {
	var samples []models.SystemState
	if err := json.Unmarshal(raw, &samples); err != nil {
		return nil
	}
	return samples
}

func (s *Spool) writeLocked(samples []models.SystemState) error {
	if len(samples) == 0 {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clear spool: %w", err)
		}
		return nil
	}

	encoded, err := json.Marshal(samples)
	if err != nil {
		return fmt.Errorf("encode spool: %w", err)
	}

	// Write to a temporary file and rename, so a crash mid-write cannot leave
	// a half-written spool behind.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return fmt.Errorf("write spool: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace spool: %w", err)
	}
	return nil
}
