package storage

import (
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func rangeStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "range.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seedEverySecond writes one sample per second starting at start.
func seedEverySecond(t *testing.T, s *Store, hostID string, start time.Time, n int) {
	t.Helper()
	batch := make([]*models.SystemState, 0, n)
	for i := range n {
		batch = append(batch, &models.SystemState{
			HostID: hostID, Hostname: hostID,
			Timestamp: start.Add(time.Duration(i) * time.Second),
			CPUUsage:  float64(i % 100),
		})
	}
	if _, err := s.SaveBatch(batch); err != nil {
		t.Fatalf("SaveBatch: %v", err)
	}
}

func TestQueryRangeReturnsOnlyTheWindow(t *testing.T) {
	s := rangeStore(t)
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	seedEverySecond(t, s, "h1", start, 600) // 10 minutes

	// A window strictly inside the seeded span.
	from := start.Add(2 * time.Minute)
	to := start.Add(4 * time.Minute)
	got, err := s.QueryRange("h1", Range{From: from, To: to}, 0)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}

	// Inclusive both ends: 120 seconds spans 121 samples.
	if len(got) != 121 {
		t.Fatalf("got %d samples, want 121 for a 2-minute window at 1s", len(got))
	}
	if got[0].Timestamp.Before(from) {
		t.Errorf("first sample %v is before the window start %v", got[0].Timestamp, from)
	}
	if got[len(got)-1].Timestamp.After(to) {
		t.Errorf("last sample %v is after the window end %v", got[len(got)-1].Timestamp, to)
	}
	// Ascending, so a chart can plot it without reversing.
	for i := 1; i < len(got); i++ {
		if got[i].Timestamp.Before(got[i-1].Timestamp) {
			t.Fatalf("samples are not in ascending order at %d", i)
		}
	}
}

func TestQueryRangeIsolatesHosts(t *testing.T) {
	s := rangeStore(t)
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	seedEverySecond(t, s, "h1", start, 60)
	seedEverySecond(t, s, "h2", start, 60)

	r := Range{From: start, To: start.Add(time.Minute)}
	got, err := s.QueryRange("h1", r, 0)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	for _, m := range got {
		if m.HostID != "h1" {
			t.Fatalf("got a sample from %q in a query scoped to h1", m.HostID)
		}
	}

	// An empty host id means every host, which is what the fleet view asks for.
	all, err := s.QueryRange("", r, 0)
	if err != nil {
		t.Fatalf("QueryRange(all): %v", err)
	}
	if len(all) <= len(got) {
		t.Errorf("all-hosts query returned %d, host-scoped returned %d", len(all), len(got))
	}
}

func TestQueryRangeRejectsInvertedWindow(t *testing.T) {
	s := rangeStore(t)
	now := time.Now()
	// A window that ends before it starts is a caller mistake, not an empty
	// result — returning nothing hides the bug.
	if _, err := s.QueryRange("", Range{From: now, To: now.Add(-time.Hour)}, 0); err == nil {
		t.Fatal("QueryRange accepted a window ending before it starts")
	}
}

func TestGetRollupsRangeHonoursBothBounds(t *testing.T) {
	s := rangeStore(t)
	// Aged past the raw window so Rollup will aggregate them.
	start := time.Now().Add(-72 * time.Hour).Truncate(time.Minute)
	seedEverySecond(t, s, "h1", start, 1800) // 30 minutes

	policy := RetentionPolicy{RawHours: 24, MinuteDays: 30, FiveMinuteDays: 365}
	if err := s.Rollup(policy, time.Now()); err != nil {
		t.Fatalf("Rollup: %v", err)
	}

	// Ten minutes out of the middle of thirty.
	from := start.Add(10 * time.Minute)
	to := start.Add(20 * time.Minute)
	got, err := s.GetRollupsRange(RollupMinute, "h1", Range{From: from, To: to}, 0)
	if err != nil {
		t.Fatalf("GetRollupsRange: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no rollup buckets in a window that has data")
	}
	if len(got) > 11 {
		t.Fatalf("got %d buckets for a 10-minute window, want at most 11 — the upper bound is not applied", len(got))
	}
	// Every bucket must fall inside the requested window, to the minute.
	for _, p := range got {
		if p.Bucket.Before(from.Truncate(time.Minute)) || p.Bucket.After(to) {
			t.Errorf("bucket %v falls outside the window %v..%v", p.Bucket, from, to)
		}
	}
}

func TestResolveResolutionPicksATierThatHasData(t *testing.T) {
	now := time.Now()
	raw := 24 * time.Hour

	cases := []struct {
		name     string
		from, to time.Time
		want     string
	}{
		{"recent short window uses raw", now.Add(-30 * time.Minute), now, ResolutionRaw},
		{"a day needs the minute tier", now.Add(-24 * time.Hour), now, RollupMinute},
		{"a month needs the five-minute tier", now.Add(-30 * 24 * time.Hour), now, RollupFiveMinute},
		// The window is short, but it is older than the raw retention, so raw
		// has already been pruned and would return nothing.
		{"old short window cannot use raw", now.Add(-72 * time.Hour), now.Add(-71 * time.Hour), RollupMinute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveResolution(Range{From: tc.from, To: tc.to}, raw, now); got != tc.want {
				t.Errorf("ResolveResolution() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQueryRangeCapsRunawayLimits(t *testing.T) {
	s := rangeStore(t)
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	seedEverySecond(t, s, "h1", start, 300)

	got, err := s.QueryRange("h1", Range{From: start, To: time.Now()}, 50)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(got) != 50 {
		t.Errorf("got %d samples with limit 50", len(got))
	}
}
