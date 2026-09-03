// Package agent implements push-mode collection: sample locally, buffer to
// disk, and forward to a sys-sentient server.
package agent

import (
	"bytes"
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
	// count caches how many lines the file holds so a routine append does not
	// have to read it. -1 means "not yet known"; it is resolved on first use.
	count int
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
	return &Spool{path: path, capacity: capacity, count: -1}, nil
}

// Append adds a sample to the buffer.
//
// One line is appended to the open file; the buffer is compacted only once it
// has drifted meaningfully past capacity. The previous implementation read,
// decoded, re-encoded and rewrote the entire file on every sample, which cost
// 230 ms per append against a full 5000-sample buffer — 11% of a 2-second poll
// interval, sustained for the whole outage the buffer exists to survive.
func (s *Spool) Append(sample models.SystemState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.count < 0 {
		samples, err := s.readLocked()
		if err != nil {
			return err
		}
		s.count = len(samples)
	}

	line, err := json.Marshal(sample)
	if err != nil {
		return fmt.Errorf("encode sample: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open spool: %w", err)
	}
	// A file left without a trailing newline — truncated by a crash or a full
	// disk — would otherwise have this sample concatenated onto its broken
	// last line, corrupting a good sample as well as the damaged one.
	terminated, err := endsWithNewline(s.path)
	if err != nil {
		_ = f.Close()
		return err
	}
	if !terminated {
		if _, err := f.Write([]byte{'\n'}); err != nil {
			_ = f.Close()
			return fmt.Errorf("terminate spool line: %w", err)
		}
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("append to spool: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close spool: %w", err)
	}
	s.count++

	// Compact lazily. Trimming on every append would reintroduce the very
	// rewrite this change removes, so the file is allowed to overshoot by a
	// tenth of its capacity and is then trimmed in one pass.
	if s.count > s.capacity+s.compactionSlack() {
		samples, err := s.readLocked()
		if err != nil {
			return err
		}
		// Drop oldest: recent data is more useful than a complete history of
		// a long outage, and the buffer must stay bounded.
		if len(samples) > s.capacity {
			samples = samples[len(samples)-s.capacity:]
		}
		return s.writeLocked(samples)
	}
	return nil
}

// endsWithNewline reports whether the spool's last byte terminates a line.
// An empty or absent file counts as terminated: there is nothing to run into.
func endsWithNewline(path string) (bool, error) {
	// #nosec G304 -- the spool's own path, from this agent's config.
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("open spool: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat spool: %w", err)
	}
	if info.Size() == 0 {
		return true, nil
	}

	last := make([]byte, 1)
	if _, err := f.ReadAt(last, info.Size()-1); err != nil {
		return false, fmt.Errorf("read spool tail: %w", err)
	}
	return last[0] == '\n', nil
}

// compactionSlack is how far past capacity the file may drift before a rewrite.
func (s *Spool) compactionSlack() int {
	slack := s.capacity / 10
	if slack < 1 {
		slack = 1
	}
	return slack
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
//
// Both formats are read: one JSON object per line, and the single JSON array
// written by versions before the append-only change — an upgrade should not
// discard whatever an agent had buffered at the time.
func decodeSpool(raw []byte) []models.SystemState {
	samples := make([]models.SystemState, 0, 64)

	// A spool written before the append-only change is a single JSON array,
	// and an upgraded agent then appends lines after it. Consume the array
	// prefix first and carry on with the remainder, or those appended samples
	// are unreachable — the file parses as neither an array nor as lines, and
	// the agent buffers forever while reporting nothing wrong.
	rest := raw
	if trimmed := bytes.TrimLeft(raw, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '[' {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		var legacy []models.SystemState
		if err := dec.Decode(&legacy); err == nil {
			samples = append(samples, legacy...)
			rest = trimmed[dec.InputOffset():]
		} else {
			// Unreadable array: drop it and try to salvage anything after it.
			if nl := bytes.IndexByte(trimmed, '\n'); nl >= 0 {
				rest = trimmed[nl+1:]
			} else {
				rest = nil
			}
		}
	}

	for line := range bytes.SplitSeq(rest, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var sample models.SystemState
		// A single corrupt line loses one sample, not the whole buffer — the
		// main reason to prefer lines over one array.
		if err := json.Unmarshal(line, &sample); err != nil {
			continue
		}
		samples = append(samples, sample)
	}

	if len(samples) == 0 {
		return nil
	}
	return samples
}

func (s *Spool) writeLocked(samples []models.SystemState) error {
	if len(samples) == 0 {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clear spool: %w", err)
		}
		s.count = 0
		return nil
	}

	var buf bytes.Buffer
	for i := range samples {
		line, err := json.Marshal(&samples[i])
		if err != nil {
			return fmt.Errorf("encode spool: %w", err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}

	// Write to a temporary file and rename, so a crash mid-write cannot leave
	// a half-written spool behind.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write spool: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace spool: %w", err)
	}
	s.count = len(samples)
	return nil
}
