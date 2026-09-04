package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sys-sentient/internal/storage"
)

// rangeQuery is a parsed ?from=&to=&resolution=&limit=&host= request.
type rangeQuery struct {
	HostID     string
	Range      storage.Range
	Resolution string
	Limit      int
	// Bounded reports whether the caller asked for a window at all. Without
	// one the handler keeps its historical "newest N" behaviour, so existing
	// clients are unaffected.
	Bounded bool
}

// parseRangeQuery reads the window parameters from a request.
//
// Errors are returned rather than silently defaulted: a mistyped timestamp
// used to become "the last 50 samples", which looks like working software
// returning the wrong answer.
func parseRangeQuery(r *http.Request, rawRetention time.Duration, now time.Time) (rangeQuery, error) {
	q := r.URL.Query()
	out := rangeQuery{HostID: q.Get("host")}

	fromRaw, toRaw := q.Get("from"), q.Get("to")
	if fromRaw != "" || toRaw != "" {
		if fromRaw == "" {
			return out, fmt.Errorf("to requires from")
		}
		from, err := parseTimestamp(fromRaw)
		if err != nil {
			return out, fmt.Errorf("from must be an RFC3339 timestamp, got %q", fromRaw)
		}
		to := now
		if toRaw != "" {
			if to, err = parseTimestamp(toRaw); err != nil {
				return out, fmt.Errorf("to must be an RFC3339 timestamp, got %q", toRaw)
			}
		}
		if to.Before(from) {
			return out, fmt.Errorf("to (%s) is before from (%s)", toRaw, fromRaw)
		}
		out.Range = storage.Range{From: from, To: to}
		out.Bounded = true
	}

	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return out, fmt.Errorf("limit must be a positive integer, got %q", raw)
		}
		if parsed > maxMetricsLimit {
			return out, fmt.Errorf("limit must be at most %d, got %d", maxMetricsLimit, parsed)
		}
		out.Limit = parsed
	}

	out.Resolution = q.Get("resolution")
	if out.Resolution == "" {
		out.Resolution = storage.ResolutionAuto
	}
	switch out.Resolution {
	case storage.ResolutionAuto:
		if out.Bounded {
			out.Resolution = storage.ResolveResolution(out.Range, rawRetention, now)
		} else {
			out.Resolution = storage.ResolutionRaw
		}
	case storage.ResolutionRaw, storage.RollupMinute, storage.RollupFiveMinute:
	default:
		return out, fmt.Errorf("resolution must be auto, raw, %s or %s, got %q",
			storage.RollupMinute, storage.RollupFiveMinute, out.Resolution)
	}

	return out, nil
}

// rawRetention reports how long unaggregated samples are kept, which decides
// whether a window can be answered from the raw tier at all.
func (s *Server) rawRetention() time.Duration {
	if s.runtime != nil {
		hours, _, _ := s.runtime.Retention()
		return time.Duration(hours) * time.Hour
	}
	return 24 * time.Hour
}

// rangeResponse is what a bounded query returns.
//
// The resolution is echoed back because "auto" is resolved server-side, and a
// chart needs to know whether it is drawing samples or averages before it
// labels an axis.
type rangeResponse struct {
	Resolution string    `json:"resolution"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Count      int       `json:"count"`
	Metrics    any       `json:"metrics"`
}

// parseTimestamp reads an RFC3339 value from a query parameter.
//
// A positive UTC offset is written "+05:30", and an unencoded "+" in a query
// string decodes to a space -- so a perfectly correct timestamp typed into curl
// arrives as "2026-09-04T17:30:57 05:30" and fails to parse. Restoring the sign
// costs nothing and saves an operator a genuinely baffling error.
func parseTimestamp(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if repaired := strings.Replace(raw, " ", "+", 1); repaired != raw {
		return time.Parse(time.RFC3339, repaired)
	}
	return time.Time{}, fmt.Errorf("not an RFC3339 timestamp: %q", raw)
}
