package storage

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/models"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "rollup.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// seed writes `n` samples one second apart ending at `end`, with a CPU value
// produced by cpuAt so a test can control the average it expects.
func seed(t *testing.T, s *Store, end time.Time, n int, cpuAt func(i int) float64) {
	t.Helper()
	for i := range n {
		if err := s.Save(&models.SystemState{
			HostID:      "host-a",
			Hostname:    "alpha",
			Timestamp:   end.Add(-time.Duration(n-i) * time.Second),
			CPUUsage:    cpuAt(i),
			MemoryUsed:  1000,
			MemoryTotal: 4000,
		}); err != nil {
			t.Fatalf("seed sample %d: %v", i, err)
		}
	}
}

// The point of the whole feature: history survives past the raw window rather
// than being deleted.
func TestRollupPreservesHistoryPastTheRawWindow(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	// 60 samples in one minute, two days ago — well past a 24h raw window.
	old := now.Add(-48 * time.Hour)
	seed(t, s, old, 60, func(i int) float64 { return float64(i) })

	policy := RetentionPolicy{RawHours: 24, MinuteDays: 30, FiveMinuteDays: 365}
	if err := s.Rollup(policy, now); err != nil {
		t.Fatalf("rollup: %v", err)
	}
	if err := s.PruneTiers(policy, now); err != nil {
		t.Fatalf("prune: %v", err)
	}

	// The raw rows are gone...
	raw, err := s.GetRecent(100)
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if len(raw) != 0 {
		t.Errorf("expected raw samples to be pruned, %d remain", len(raw))
	}

	// ...but the history is not.
	points, err := s.GetRollups(RollupMinute, "host-a", old.Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("read rollups: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("history was destroyed rather than rolled up")
	}

	var samples int
	for _, p := range points {
		samples += p.Samples
	}
	if samples != 60 {
		t.Errorf("rolled up %d samples, want 60", samples)
	}
}

// Running the rollup twice must not double-count. A restart mid-pass, or an
// overlapping maintenance tick, would otherwise corrupt every average.
func TestRollupIsIdempotent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	seed(t, s, old, 30, func(int) float64 { return 50 })

	policy := DefaultRetention()
	for range 3 {
		if err := s.Rollup(policy, now); err != nil {
			t.Fatalf("rollup: %v", err)
		}
	}

	points, err := s.GetRollups(RollupMinute, "host-a", old.Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	total := 0
	for _, p := range points {
		total += p.Samples
		if math.Abs(p.CPUAvg-50) > 0.001 {
			t.Errorf("bucket %s average drifted to %.4f after repeated rollups", p.Bucket, p.CPUAvg)
		}
	}
	if total != 30 {
		t.Errorf("after three rollups the tier holds %d samples, want 30", total)
	}
}

// Folding minute buckets into five-minute ones means averaging averages, which
// is only correct if each input is weighted by how many samples it represents.
// An unweighted mean of buckets holding 1 and 59 samples is badly wrong.
func TestFiveMinuteRollupWeightsBySampleCount(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	// Two minutes in the same five-minute bucket, 40 days ago so both tiers
	// are exercised. One busy minute of 59 samples at 100%, one quiet minute
	// of a single sample at 0%.
	base := now.AddDate(0, 0, -40).Truncate(time.Hour)
	for i := range 59 {
		if err := s.Save(&models.SystemState{
			HostID: "host-a", Hostname: "alpha",
			Timestamp: base.Add(time.Duration(i) * time.Second),
			CPUUsage:  100, MemoryTotal: 1,
		}); err != nil {
			t.Fatalf("seed busy: %v", err)
		}
	}
	if err := s.Save(&models.SystemState{
		HostID: "host-a", Hostname: "alpha",
		Timestamp: base.Add(70 * time.Second),
		CPUUsage:  0, MemoryTotal: 1,
	}); err != nil {
		t.Fatalf("seed quiet: %v", err)
	}

	policy := RetentionPolicy{RawHours: 24, MinuteDays: 30, FiveMinuteDays: 365}
	if err := s.Rollup(policy, now); err != nil {
		t.Fatalf("rollup: %v", err)
	}

	points, err := s.GetRollups(RollupFiveMinute, "host-a", base.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("no five-minute bucket produced")
	}

	// Weighted: (100*59 + 0*1) / 60 = 98.33. Unweighted would give 50.
	const want = 98.333
	if math.Abs(points[0].CPUAvg-want) > 0.1 {
		t.Errorf("CPU average %.3f, want %.3f — averages are not weighted by sample count",
			points[0].CPUAvg, want)
	}
	if points[0].CPUMax != 100 {
		t.Errorf("peak lost in the fold: max=%.1f, want 100", points[0].CPUMax)
	}
}

// Pruning must never run ahead of the rollup, or the samples it deletes were
// never aggregated and the history is simply gone.
func TestPruneDoesNotOutrunRollup(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	seed(t, s, old, 10, func(int) float64 { return 25 })

	policy := DefaultRetention()
	// Deliberately prune with no prior rollup, as a buggy caller would.
	if err := s.PruneTiers(policy, now); err != nil {
		t.Fatalf("prune: %v", err)
	}
	points, err := s.GetRollups(RollupMinute, "host-a", old.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(points) != 0 {
		t.Fatal("unexpected rollups: this test asserts the hazard, not the fix")
	}
	// Documented consequence: the caller must Rollup before PruneTiers. The
	// daemon's maintenance tick does exactly that, and this test exists so
	// reordering them is a visible decision rather than a silent data loss.
}
