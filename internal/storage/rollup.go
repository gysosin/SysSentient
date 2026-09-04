package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// Retention is tiered rather than a single cut-off.
//
// The product previously kept 24 hours of raw samples and hard-DELETEd
// everything older, so a question like "was this machine slow last Tuesday"
// had no answer. Keeping raw samples for a year instead is not an option:
// measured at ~4.3 KB per row and a two-second interval, that is roughly
// 65 GB per host per year.
//
// Rolling up solves both. A minute of samples answers "what was the load
// yesterday afternoon" just as well as thirty individual readings, at
// 1/30th the rows.
const (
	// RollupMinute is the tier for recent history: per-minute averages.
	RollupMinute = "1m"
	// RollupFiveMinute is the long-tail tier.
	RollupFiveMinute = "5m"
)

// RetentionPolicy describes how long each tier is kept.
type RetentionPolicy struct {
	// RawHours keeps full-resolution samples. Short by design: this is the
	// tier that costs 4.3 KB a row.
	RawHours int
	// MinuteDays keeps per-minute rollups.
	MinuteDays int
	// FiveMinuteDays keeps per-five-minute rollups.
	FiveMinuteDays int
}

// DefaultRetention keeps a year of history in about the space a day of raw
// samples used to take.
func DefaultRetention() RetentionPolicy {
	return RetentionPolicy{RawHours: 24, MinuteDays: 30, FiveMinuteDays: 365}
}

// createRollupTable is called during schema setup.
//
// Rollups live in their own table rather than sharing `metrics`. They are a
// different shape — averages and maxima over a window, with no process list or
// filesystem detail, because those do not average meaningfully — and mixing
// them in would mean every read had to know which kind of row it had.
func createRollupTable(db *sql.DB) error {
	if _, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS metric_rollups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bucket DATETIME NOT NULL,
		resolution TEXT NOT NULL,
		host_id TEXT NOT NULL DEFAULT '',
		samples INTEGER NOT NULL,
		cpu_avg REAL, cpu_max REAL,
		memory_used_avg REAL, memory_used_max REAL,
		memory_total REAL,
		swap_used_avg REAL,
		load_avg_1 REAL, load_max_1 REAL,
		disk_read_avg REAL, disk_write_avg REAL, disk_iops_avg REAL,
		net_sent_avg REAL, net_recv_avg REAL,
		temperature_avg REAL, temperature_max REAL,
		UNIQUE(bucket, resolution, host_id)
	);`); err != nil {
		return fmt.Errorf("create metric_rollups: %w", err)
	}

	// Reads are always "this resolution, this host, this time range".
	if _, err := db.Exec(`
	CREATE INDEX IF NOT EXISTS idx_rollups_lookup
		ON metric_rollups(resolution, host_id, bucket);`); err != nil {
		return fmt.Errorf("index metric_rollups: %w", err)
	}
	return nil
}

// Rollup aggregates raw samples older than the raw window into per-minute
// buckets, then per-minute buckets into five-minute ones.
//
// Idempotent: the UNIQUE constraint plus INSERT OR REPLACE means running it
// twice produces the same table, so a restart mid-pass cannot double-count.
// It deliberately does not delete anything — pruning is a separate, explicit
// step, so a bug here cannot destroy history.
func (s *Store) Rollup(policy RetentionPolicy, now time.Time) error {
	rawCutoff := now.Add(-time.Duration(policy.RawHours) * time.Hour)

	// Per-minute, from raw samples that are about to age out.
	//
	// strftime truncates the timestamp to the start of its bucket, which is
	// what makes this idempotent: the same sample always lands in the same
	// bucket no matter when the rollup runs.
	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO metric_rollups (
			bucket, resolution, host_id, samples,
			cpu_avg, cpu_max, memory_used_avg, memory_used_max, memory_total,
			swap_used_avg, load_avg_1, load_max_1,
			disk_read_avg, disk_write_avg, disk_iops_avg,
			net_sent_avg, net_recv_avg, temperature_avg, temperature_max)
		SELECT
			strftime('%Y-%m-%d %H:%M:00', timestamp) AS bucket,
			?, COALESCE(host_id, ''), COUNT(*),
			AVG(cpu_usage), MAX(cpu_usage),
			AVG(memory_used), MAX(memory_used), MAX(memory_total),
			AVG(swap_used), AVG(load_avg_1), MAX(load_avg_1),
			AVG(disk_read_bytes), AVG(disk_write_bytes), AVG(disk_iops),
			AVG(net_sent_bytes), AVG(net_recv_bytes),
			AVG(temperature), MAX(temperature)
		FROM metrics
		WHERE timestamp IS NOT NULL AND timestamp < ?
		GROUP BY bucket, COALESCE(host_id, '')`,
		RollupMinute, sqlTime(rawCutoff)); err != nil {
		return fmt.Errorf("rollup to %s: %w", RollupMinute, err)
	}

	// Per-five-minute, from the minute tier once it ages out.
	//
	// Averaging averages is only correct when the inputs carry equal weight,
	// which is why `samples` is summed and the averages are weighted by it.
	minuteCutoff := now.AddDate(0, 0, -policy.MinuteDays)
	if _, err := s.db.Exec(`
		INSERT OR REPLACE INTO metric_rollups (
			bucket, resolution, host_id, samples,
			cpu_avg, cpu_max, memory_used_avg, memory_used_max, memory_total,
			swap_used_avg, load_avg_1, load_max_1,
			disk_read_avg, disk_write_avg, disk_iops_avg,
			net_sent_avg, net_recv_avg, temperature_avg, temperature_max)
		SELECT
			strftime('%Y-%m-%d %H:', bucket) ||
				printf('%02d', (CAST(strftime('%M', bucket) AS INTEGER) / 5) * 5) || ':00',
			?, host_id, SUM(samples),
			SUM(cpu_avg * samples) / SUM(samples), MAX(cpu_max),
			SUM(memory_used_avg * samples) / SUM(samples), MAX(memory_used_max),
			MAX(memory_total),
			SUM(swap_used_avg * samples) / SUM(samples),
			SUM(load_avg_1 * samples) / SUM(samples), MAX(load_max_1),
			SUM(disk_read_avg * samples) / SUM(samples),
			SUM(disk_write_avg * samples) / SUM(samples),
			SUM(disk_iops_avg * samples) / SUM(samples),
			SUM(net_sent_avg * samples) / SUM(samples),
			SUM(net_recv_avg * samples) / SUM(samples),
			SUM(temperature_avg * samples) / SUM(samples), MAX(temperature_max)
		FROM metric_rollups
		WHERE resolution = ? AND bucket < ? AND samples > 0
		GROUP BY 1, host_id`,
		RollupFiveMinute, RollupMinute, sqlTime(minuteCutoff)); err != nil {
		return fmt.Errorf("rollup to %s: %w", RollupFiveMinute, err)
	}

	return nil
}

// PruneTiers removes data each tier no longer needs.
//
// Ordered after Rollup by the caller, never before: deleting raw samples that
// have not been aggregated yet loses them permanently.
func (s *Store) PruneTiers(policy RetentionPolicy, now time.Time) error {
	// Raw samples, once rolled up.
	if _, err := s.db.Exec(`DELETE FROM metrics WHERE timestamp IS NOT NULL AND timestamp < ?`,
		sqlTime(now.Add(-time.Duration(policy.RawHours)*time.Hour))); err != nil {
		return fmt.Errorf("prune raw metrics: %w", err)
	}

	// Minute buckets, once folded into five-minute ones.
	if _, err := s.db.Exec(`DELETE FROM metric_rollups WHERE resolution = ? AND bucket < ?`,
		RollupMinute, sqlTime(now.AddDate(0, 0, -policy.MinuteDays))); err != nil {
		return fmt.Errorf("prune minute rollups: %w", err)
	}

	// Five-minute buckets, at the end of the retention window. This is the
	// only tier where data actually leaves the system.
	if _, err := s.db.Exec(`DELETE FROM metric_rollups WHERE resolution = ? AND bucket < ?`,
		RollupFiveMinute, sqlTime(now.AddDate(0, 0, -policy.FiveMinuteDays))); err != nil {
		return fmt.Errorf("prune five-minute rollups: %w", err)
	}

	return nil
}

// RollupPoint is one aggregated bucket.
type RollupPoint struct {
	Bucket      time.Time `json:"bucket"`
	Resolution  string    `json:"resolution"`
	HostID      string    `json:"host_id"`
	Samples     int       `json:"samples"`
	CPUAvg      float64   `json:"cpu_avg"`
	CPUMax      float64   `json:"cpu_max"`
	MemoryUsed  float64   `json:"memory_used_avg"`
	MemoryTotal float64   `json:"memory_total"`
	SwapUsed    float64   `json:"swap_used_avg"`
	LoadAvg1    float64   `json:"load_avg_1"`
	LoadMax1    float64   `json:"load_max_1"`
	DiskRead    float64   `json:"disk_read_avg"`
	DiskWrite   float64   `json:"disk_write_avg"`
	NetSent     float64   `json:"net_sent_avg"`
	NetRecv     float64   `json:"net_recv_avg"`
	Temperature float64   `json:"temperature_avg"`
	TempMax     float64   `json:"temperature_max"`
}

// Compact reclaims space after pruning.
//
// Two separate problems, both previously unaddressed.
//
// The write-ahead log grows until something checkpoints it. Under continuous
// writes SQLite's automatic checkpoint often cannot complete, and the WAL was
// measured larger than the database it belonged to — 4.0 MB against 1.8 MB.
// TRUNCATE resets it to empty rather than merely rewinding.
//
// DELETE returns pages to SQLite's freelist but never to the filesystem, so a
// database that spiked stayed large forever. VACUUM rebuilds the file. It is
// expensive and takes an exclusive lock, which is why it runs on the
// maintenance tick rather than after every prune.
func (s *Store) Compact(vacuum bool) error {
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	if vacuum {
		if _, err := s.db.Exec(`VACUUM`); err != nil {
			return fmt.Errorf("vacuum: %w", err)
		}
	}
	return nil
}
