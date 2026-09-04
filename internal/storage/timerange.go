package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"sys-sentient/internal/models"
)

// ResolutionRaw selects unaggregated samples. The rollup tiers use their own
// constants, RollupMinute and RollupFiveMinute.
const ResolutionRaw = "raw"

// ResolutionAuto asks the server to pick a tier from the window.
const ResolutionAuto = "auto"

// maxRangeRows bounds a single range query.
//
// A year of five-minute buckets is about 105,000 rows and a day of raw samples
// at a two-second interval is 43,200; neither is plottable, and materialising
// them costs the server more than the caller can use.
const maxRangeRows = 20000

// defaultRangeRows is the cap when a caller does not ask for one. Comfortably
// more than a chart needs at any sensible width.
const defaultRangeRows = 2000

// ErrInvalidRange reports a window that ends before it begins.
var ErrInvalidRange = errors.New("range ends before it begins")

// Range is a closed time window: both ends are included.
type Range struct {
	From time.Time
	To   time.Time
}

// Duration is how much time the window spans.
func (r Range) Duration() time.Duration { return r.To.Sub(r.From) }

func (r Range) validate() error {
	if r.To.Before(r.From) {
		return fmt.Errorf("%w: %s to %s", ErrInvalidRange,
			r.From.Format(time.RFC3339), r.To.Format(time.RFC3339))
	}
	return nil
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultRangeRows
	case limit > maxRangeRows:
		return maxRangeRows
	default:
		return limit
	}
}

// ResolveResolution picks the storage tier that can actually answer a window.
//
// Span alone is not enough. A one-hour window from three days ago is short, but
// raw samples that old have already been pruned, so answering it from the raw
// tier returns nothing at all. The window's age decides first, then its width.
func ResolveResolution(r Range, rawRetention time.Duration, now time.Time) string {
	// A margin below the retention edge: a window that ends just inside it is
	// about to be pruned mid-query, and half an answer is worse than a
	// coarser whole one.
	rawHorizon := now.Add(-rawRetention + time.Hour)

	if r.From.After(rawHorizon) && r.Duration() <= time.Hour {
		return ResolutionRaw
	}
	if r.Duration() <= 36*time.Hour {
		return RollupMinute
	}
	return RollupFiveMinute
}

// QueryRange returns raw samples inside a window, oldest first.
//
// This is the read the dashboard never had: every other raw accessor is
// "newest N", which is why the console could only ever show the last few
// minutes no matter how much history the database held.
func (s *Store) QueryRange(hostID string, r Range, limit int) ([]models.SystemState, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	query := `
		SELECT timestamp, cpu_usage, COALESCE(cpu_per_core, '[]'), memory_used, memory_total,
		       COALESCE(swap_used, 0), COALESCE(swap_total, 0),
		       disk_read_bytes, disk_write_bytes, COALESCE(disk_iops, 0),
		       net_sent_bytes, net_recv_bytes,
		       COALESCE(load_avg_1, 0), COALESCE(load_avg_5, 0), COALESCE(load_avg_15, 0),
		       temperature, top_processes, COALESCE(processes, '[]'),
		       COALESCE(uptime_seconds, 0), COALESCE(hostname, ''),
		       COALESCE(filesystems, '[]'), COALESCE(host_id, ''),
		       COALESCE(memory_cached, 0), COALESCE(memory_buffers, 0)
		FROM metrics
		WHERE timestamp >= ? AND timestamp <= ?`
	args := []any{sqlTime(r.From), sqlTime(r.To)}
	if hostID != "" {
		query += ` AND host_id = ?`
		args = append(args, hostID)
	}
	// Ascending, so callers plot without reversing. The newest-N readers sort
	// descending because they are answering "what is happening now".
	query += ` ORDER BY timestamp ASC LIMIT ?`
	args = append(args, clampLimit(limit))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanMetricRows(rows)
}

// GetRollupsRange returns aggregated buckets inside a window.
//
// GetRollups is the same query with no upper bound; keeping both would invite
// the half-open version back, so callers that want "everything since X" pass a
// To of time.Now().
func (s *Store) GetRollupsRange(resolution, hostID string, r Range, limit int) ([]RollupPoint, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}

	query := `
		SELECT bucket, resolution, host_id, samples,
		       COALESCE(cpu_avg,0), COALESCE(cpu_max,0),
		       COALESCE(memory_used_avg,0), COALESCE(memory_total,0),
		       COALESCE(swap_used_avg,0),
		       COALESCE(load_avg_1,0), COALESCE(load_max_1,0),
		       COALESCE(disk_read_avg,0), COALESCE(disk_write_avg,0),
		       COALESCE(net_sent_avg,0), COALESCE(net_recv_avg,0),
		       COALESCE(temperature_avg,0), COALESCE(temperature_max,0)
		FROM metric_rollups
		WHERE resolution = ? AND bucket >= ? AND bucket <= ?`
	args := []any{resolution, sqlTime(r.From), sqlTime(r.To)}
	if hostID != "" {
		query += ` AND host_id = ?`
		args = append(args, hostID)
	}
	query += ` ORDER BY bucket ASC LIMIT ?`
	args = append(args, clampLimit(limit))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanRollupRows(rows)
}

// scanMetricRows decodes a metrics result set.
//
// Shared by every reader so the column list, the JSON decoding and the derived
// top-processes summary cannot drift between "newest N" and "this window".
func scanMetricRows(rows *sql.Rows) ([]models.SystemState, error) {
	results := make([]models.SystemState, 0, 64)
	for rows.Next() {
		var m models.SystemState
		var cpuPerCoreJSON, processesJSON, filesystemsJSON string
		if err := rows.Scan(
			&m.Timestamp, &m.CPUUsage, &cpuPerCoreJSON,
			&m.MemoryUsed, &m.MemoryTotal, &m.SwapUsed, &m.SwapTotal,
			&m.DiskReadBytes, &m.DiskWriteBytes, &m.DiskIOPS,
			&m.NetSentBytes, &m.NetRecvBytes, &m.LoadAvg1, &m.LoadAvg5, &m.LoadAvg15,
			&m.Temperature, &m.TopProcesses, &processesJSON, &m.UptimeSeconds,
			&m.Hostname, &filesystemsJSON, &m.HostID,
			&m.MemoryCached, &m.MemoryBuffers,
		); err != nil {
			return nil, err
		}
		m.CPUPerCore = decodeCPUPerCore(cpuPerCoreJSON)
		m.Processes = decodeProcesses(processesJSON)
		m.Filesystems = decodeFilesystems(filesystemsJSON)
		restoreTopProcesses(&m)
		results = append(results, m)
	}
	return results, rows.Err()
}

// scanRollupRows decodes a metric_rollups result set.
func scanRollupRows(rows *sql.Rows) ([]RollupPoint, error) {
	points := make([]RollupPoint, 0, 64)
	for rows.Next() {
		var p RollupPoint
		if err := rows.Scan(&p.Bucket, &p.Resolution, &p.HostID, &p.Samples,
			&p.CPUAvg, &p.CPUMax, &p.MemoryUsed, &p.MemoryTotal, &p.SwapUsed,
			&p.LoadAvg1, &p.LoadMax1, &p.DiskRead, &p.DiskWrite,
			&p.NetSent, &p.NetRecv, &p.Temperature, &p.TempMax); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}
