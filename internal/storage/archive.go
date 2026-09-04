package storage

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ArchiveResult reports what one archiving pass did.
type ArchiveResult struct {
	Path       string `json:"path"`
	Rows       int    `json:"rows"`
	Bytes      int64  `json:"bytes"`
	Resolution string `json:"resolution"`
}

// ArchiveTier writes rollup rows older than a cutoff to a compressed file and
// removes them from the database.
//
// Retention deletes; this keeps. The two are different needs: an operator who
// sets a year of retention is choosing what the dashboard can query, not
// consenting to lose everything older. Archived rows leave the database — so
// the file stops growing — while staying restorable.
//
// Written and fsynced before anything is deleted. A crash between the two
// leaves a duplicate archive, which is recoverable; the other order loses data.
func (s *Store) ArchiveTier(resolution, dir string, before time.Time) (ArchiveResult, error) {
	result := ArchiveResult{Resolution: resolution}

	if dir == "" {
		return result, fmt.Errorf("no archive directory configured")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return result, fmt.Errorf("create archive directory: %w", err)
	}

	points, err := s.GetRollupsRange(resolution, "",
		Range{From: time.Unix(0, 0), To: before}, maxRangeRows)
	if err != nil {
		return result, fmt.Errorf("read %s tier: %w", resolution, err)
	}
	if len(points) == 0 {
		return result, nil
	}

	name := fmt.Sprintf("sys-sentient_%s_%s.jsonl.gz",
		resolution, before.UTC().Format("20060102T150405"))
	path := filepath.Join(dir, name)

	// A temporary file renamed into place, so a reader never sees a partial
	// archive and a crash mid-write leaves nothing to mistake for a complete
	// one.
	tmp := path + ".tmp"
	if err := writeArchive(tmp, points); err != nil {
		_ = os.Remove(tmp)
		return result, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return result, fmt.Errorf("finalise archive: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return result, fmt.Errorf("stat archive: %w", err)
	}

	// Only now, with the archive durable on disk, remove the rows.
	if _, err := s.db.Exec(
		`DELETE FROM metric_rollups WHERE resolution = ? AND bucket <= ?`,
		resolution, sqlTime(before)); err != nil {
		return result, fmt.Errorf("prune archived rows: %w", err)
	}

	result.Path, result.Rows, result.Bytes = path, len(points), info.Size()
	return result, nil
}

// writeArchive streams rows as gzipped JSON lines.
//
// One object per line rather than one big array: an archive can be inspected
// with zcat and grep, appended to, and read back without holding all of it in
// memory.
func writeArchive(path string, points []RollupPoint) error {
	// 0600: the archive holds the same metrics the database does.
	// #nosec G304 -- path is built from the operator's configured archive
	// directory and a name this function generates; nothing external reaches it.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	enc := json.NewEncoder(gz)
	for i := range points {
		if err := enc.Encode(&points[i]); err != nil {
			return fmt.Errorf("write archive: %w", err)
		}
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close archive stream: %w", err)
	}
	// fsync before the caller deletes anything: a rename is not enough on its
	// own if the machine loses power before the data reaches the disk.
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync archive: %w", err)
	}
	return nil
}

// RestoreArchive reads an archive back into the database.
//
// Idempotent, because the rollup table's unique constraint means a bucket
// already present is replaced rather than duplicated — so a restore run twice,
// or overlapping an existing range, is safe.
func (s *Store) RestoreArchive(path string) (int, error) {
	// #nosec G304 -- the path is the operator's own argument.
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("read archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO metric_rollups (
			bucket, resolution, host_id, samples,
			cpu_avg, cpu_max, memory_used_avg, memory_total, swap_used_avg,
			load_avg_1, load_max_1, disk_read_avg, disk_write_avg,
			net_sent_avg, net_recv_avg, temperature_avg, temperature_max)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	dec := json.NewDecoder(gz)
	restored := 0
	for dec.More() {
		var p RollupPoint
		if err := dec.Decode(&p); err != nil {
			return restored, fmt.Errorf("decode archive row %d: %w", restored+1, err)
		}
		if _, err := stmt.Exec(sqlTime(p.Bucket), p.Resolution, p.HostID, p.Samples,
			p.CPUAvg, p.CPUMax, p.MemoryUsed, p.MemoryTotal, p.SwapUsed,
			p.LoadAvg1, p.LoadMax1, p.DiskRead, p.DiskWrite,
			p.NetSent, p.NetRecv, p.Temperature, p.TempMax); err != nil {
			return restored, fmt.Errorf("restore row %d: %w", restored+1, err)
		}
		restored++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return restored, nil
}

// DiskUsage reports what the database currently occupies.
type DiskUsage struct {
	DatabaseBytes int64 `json:"database_bytes"`
	WALBytes      int64 `json:"wal_bytes"`
	RawRows       int   `json:"raw_rows"`
	RollupRows    int   `json:"rollup_rows"`
	// BytesPerRawRow lets the dashboard project the cost of a retention change
	// before it is applied, instead of after.
	BytesPerRawRow float64 `json:"bytes_per_raw_row"`
}

// Usage measures the database on disk.
func (s *Store) Usage(path string) (DiskUsage, error) {
	var u DiskUsage
	if info, err := os.Stat(path); err == nil {
		u.DatabaseBytes = info.Size()
	}
	// The write-ahead log is part of what the database costs, and under
	// continuous writes it is not small.
	if info, err := os.Stat(path + "-wal"); err == nil {
		u.WALBytes = info.Size()
	}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM metrics`).Scan(&u.RawRows); err != nil {
		return u, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM metric_rollups`).Scan(&u.RollupRows); err != nil {
		return u, err
	}
	if u.RawRows > 0 {
		u.BytesPerRawRow = float64(u.DatabaseBytes) / float64(u.RawRows)
	}
	return u, nil
}
