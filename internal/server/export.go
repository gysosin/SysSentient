package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"sys-sentient/internal/storage"
)

// maxExportRows bounds a single export.
//
// Without a ceiling, one request for a year of per-minute data would stream
// half a million rows out of a database the daemon also needs for its own
// two-second write, on a machine the product exists to avoid disturbing.
const maxExportRows = 50000

// handleExport serves the retained history as CSV or JSON.
//
// Tiered retention keeps a year of data; without a way to get it out, that
// history is trapped in one SQLite file on one host. This is the way out.
//
//	GET /api/export?format=csv&resolution=1m&since=2026-01-01T00:00:00Z
//
// Resolution selects the tier: "raw" for full-resolution samples, "1m" or "5m"
// for the rollups.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	format := q.Get("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		http.Error(w, `format must be "json" or "csv"`, http.StatusBadRequest)
		return
	}

	resolution := q.Get("resolution")
	if resolution == "" {
		resolution = storage.RollupMinute
	}

	since := time.Now().AddDate(0, 0, -7)
	if raw := q.Get("since"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "since must be an RFC3339 timestamp", http.StatusBadRequest)
			return
		}
		since = parsed
	}

	limit := maxExportRows
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		limit = min(parsed, maxExportRows)
	}

	// An upper bound, so a window can be exported rather than only an
	// open-ended tail. Absent means "up to now", which is what callers who
	// only pass since already expect.
	until := time.Now()
	if raw := q.Get("until"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "until must be an RFC3339 timestamp", http.StatusBadRequest)
			return
		}
		until = parsed
	}
	if until.Before(since) {
		http.Error(w, "until is before since", http.StatusBadRequest)
		return
	}
	window := storage.Range{From: since, To: until}

	hostID := q.Get("host")

	if resolution == storage.ResolutionRaw {
		s.exportRaw(w, format, hostID, window, limit)
		return
	}

	points, err := s.store.GetRollupsRange(resolution, hostID, window, limit)
	if err != nil {
		http.Error(w, "failed to read history", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("sys-sentient_%s_%s.%s", resolution, time.Now().UTC().Format("20060102"), format)
	// attachment, so a browser saves the file rather than rendering a wall of
	// CSV it cannot do anything with.
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		// Never null: an empty range is an empty list, and a client that has
		// to special-case null for "no rows" will eventually forget to.
		if points == nil {
			points = []storage.RollupPoint{}
		}
		_ = json.NewEncoder(w).Encode(points)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{
		"bucket", "resolution", "host_id", "samples",
		"cpu_avg", "cpu_max", "memory_used_avg", "memory_total", "swap_used_avg",
		"load_avg_1", "load_max_1", "disk_read_avg", "disk_write_avg",
		"net_sent_avg", "net_recv_avg", "temperature_avg", "temperature_max",
	})
	for _, p := range points {
		_ = cw.Write([]string{
			p.Bucket.UTC().Format(time.RFC3339), p.Resolution, p.HostID, strconv.Itoa(p.Samples),
			f(p.CPUAvg), f(p.CPUMax), f(p.MemoryUsed), f(p.MemoryTotal), f(p.SwapUsed),
			f(p.LoadAvg1), f(p.LoadMax1), f(p.DiskRead), f(p.DiskWrite),
			f(p.NetSent), f(p.NetRecv), f(p.Temperature), f(p.TempMax),
		})
	}
}

// exportRaw streams unaggregated samples for a window.
//
// It used to ignore the window entirely and return the newest N samples, so
// asking for last Tuesday quietly gave you today.
func (s *Server) exportRaw(w http.ResponseWriter, format, hostID string, window storage.Range, limit int) {
	samples, err := s.store.QueryRange(hostID, window, limit)
	if err != nil {
		http.Error(w, "failed to read samples", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("sys-sentient_raw_%s.%s", time.Now().UTC().Format("20060102"), format)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(samples)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// The process and filesystem lists are deliberately omitted: they are
	// nested JSON, and a CSV cell containing a serialised array is not
	// something a spreadsheet can use. Ask for JSON if those are wanted.
	_ = cw.Write([]string{
		"timestamp", "host_id", "hostname", "cpu_usage",
		"memory_used", "memory_total", "swap_used", "swap_total",
		"load_avg_1", "load_avg_5", "load_avg_15", "temperature", "uptime_seconds",
	})
	for _, m := range samples {
		_ = cw.Write([]string{
			m.Timestamp.UTC().Format(time.RFC3339), m.HostID, m.Hostname, f(m.CPUUsage),
			strconv.FormatUint(m.MemoryUsed, 10), strconv.FormatUint(m.MemoryTotal, 10),
			strconv.FormatUint(m.SwapUsed, 10), strconv.FormatUint(m.SwapTotal, 10),
			f(m.LoadAvg1), f(m.LoadAvg5), f(m.LoadAvg15), f(m.Temperature),
			strconv.FormatUint(m.UptimeSeconds, 10),
		})
	}
}

// f formats a float for CSV without scientific notation, which spreadsheets
// import as text.
func f(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}
