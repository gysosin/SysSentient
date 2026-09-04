package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedRollups ages samples past the raw window and rolls them up.
func seedRollups(t *testing.T, s *Store, hostID string, start time.Time, n int) {
	t.Helper()
	seedEverySecond(t, s, hostID, start, n)
	if err := s.Rollup(RetentionPolicy{RawHours: 24, MinuteDays: 30, FiveMinuteDays: 365}, time.Now()); err != nil {
		t.Fatalf("Rollup: %v", err)
	}
}

func TestArchiveRoundTrip(t *testing.T) {
	s := rangeStore(t)
	dir := t.TempDir()
	start := time.Now().Add(-72 * time.Hour).Truncate(time.Minute)
	seedRollups(t, s, "h1", start, 1800)

	before := time.Now().Add(-48 * time.Hour)
	res, err := s.ArchiveTier(RollupMinute, dir, before)
	if err != nil {
		t.Fatalf("ArchiveTier: %v", err)
	}
	if res.Rows == 0 {
		t.Fatal("archived nothing")
	}
	if res.Bytes == 0 {
		t.Error("archive file is empty")
	}

	// The rows must be gone from the database, or archiving has not helped
	// the disk at all.
	remaining, err := s.GetRollupsRange(RollupMinute, "", Range{From: time.Unix(0, 0), To: before}, 100)
	if err != nil {
		t.Fatalf("GetRollupsRange: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("%d archived rows are still in the database", len(remaining))
	}

	// ...and must come back.
	restored, err := s.RestoreArchive(res.Path)
	if err != nil {
		t.Fatalf("RestoreArchive: %v", err)
	}
	if restored != res.Rows {
		t.Errorf("restored %d rows, archived %d", restored, res.Rows)
	}

	back, err := s.GetRollupsRange(RollupMinute, "", Range{From: time.Unix(0, 0), To: before}, 5000)
	if err != nil {
		t.Fatalf("GetRollupsRange after restore: %v", err)
	}
	if len(back) != res.Rows {
		t.Errorf("after restore the tier holds %d rows, want %d", len(back), res.Rows)
	}
}

func TestRestoreIsIdempotent(t *testing.T) {
	s := rangeStore(t)
	dir := t.TempDir()
	start := time.Now().Add(-72 * time.Hour).Truncate(time.Minute)
	seedRollups(t, s, "h1", start, 600)

	res, err := s.ArchiveTier(RollupMinute, dir, time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("ArchiveTier: %v", err)
	}

	// Running a restore twice, or over a range that already exists, must not
	// duplicate anything — the unique constraint replaces instead.
	for range 2 {
		if _, err := s.RestoreArchive(res.Path); err != nil {
			t.Fatalf("RestoreArchive: %v", err)
		}
	}
	back, err := s.GetRollupsRange(RollupMinute, "", Range{From: time.Unix(0, 0), To: time.Now()}, 5000)
	if err != nil {
		t.Fatalf("GetRollupsRange: %v", err)
	}
	if len(back) != res.Rows {
		t.Errorf("after two restores the tier holds %d rows, want %d", len(back), res.Rows)
	}
}

func TestArchiveWritesBeforeItDeletes(t *testing.T) {
	s := rangeStore(t)
	dir := t.TempDir()
	start := time.Now().Add(-72 * time.Hour).Truncate(time.Minute)
	seedRollups(t, s, "h1", start, 600)

	res, err := s.ArchiveTier(RollupMinute, dir, time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("ArchiveTier: %v", err)
	}

	// The file must exist and be complete on disk. Deleting first and writing
	// second would lose data on a crash between the two.
	info, err := os.Stat(res.Path)
	if err != nil {
		t.Fatalf("archive missing: %v", err)
	}
	if info.Size() == 0 {
		t.Error("archive exists but is empty")
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("archive mode = %04o, want 0600 — it holds the same metrics the database does", perm)
	}
	// No temporary file left behind to be mistaken for a complete archive.
	if _, err := os.Stat(res.Path + ".tmp"); !os.IsNotExist(err) {
		t.Error("a .tmp file survived a successful archive")
	}
}

func TestArchiveRefusesWithoutADirectory(t *testing.T) {
	s := rangeStore(t)
	if _, err := s.ArchiveTier(RollupMinute, "", time.Now()); err == nil {
		t.Fatal("archiving succeeded with no directory configured")
	}
}

func TestArchiveOfAnEmptyTierIsANoOp(t *testing.T) {
	s := rangeStore(t)
	res, err := s.ArchiveTier(RollupMinute, t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("ArchiveTier: %v", err)
	}
	if res.Rows != 0 || res.Path != "" {
		t.Errorf("empty tier produced %+v; an archive of nothing should be nothing", res)
	}
	// And no stray file.
	entries, _ := filepath.Glob(filepath.Join(t.TempDir(), "*"))
	if len(entries) != 0 {
		t.Errorf("wrote %d files for an empty tier", len(entries))
	}
}

func TestUsageReportsWhatTheDatabaseCosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer func() { _ = s.Close() }()

	seedEverySecond(t, s, "h1", time.Now().Add(-time.Hour), 500)

	u, err := s.Usage(path)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.RawRows != 500 {
		t.Errorf("RawRows = %d, want 500", u.RawRows)
	}
	if u.DatabaseBytes == 0 {
		t.Error("DatabaseBytes is zero for a database with data in it")
	}
	// The per-row figure is what lets the dashboard price a retention change
	// before it is applied rather than after.
	if u.BytesPerRawRow <= 0 {
		t.Errorf("BytesPerRawRow = %v", u.BytesPerRawRow)
	}
}

// TestArchivingActuallyReclaimsDisk is the point of the whole shard: retention
// alone deletes, and a database that only grows is the problem an operator
// reported. This asserts the file gets smaller, not merely that rows move.
func TestArchivingActuallyReclaimsDisk(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "big.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer func() { _ = s.Close() }()

	// A month of one-second samples, aged past the raw window.
	start := time.Now().Add(-60 * 24 * time.Hour).Truncate(time.Minute)
	for i := range 20 {
		seedEverySecond(t, s, "h1", start.Add(time.Duration(i)*3*time.Hour), 3000)
	}
	if err := s.Rollup(RetentionPolicy{RawHours: 24, MinuteDays: 30, FiveMinuteDays: 365}, time.Now()); err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if err := s.PruneTiers(RetentionPolicy{RawHours: 24, MinuteDays: 3650, FiveMinuteDays: 3650}, time.Now()); err != nil {
		t.Fatalf("PruneTiers: %v", err)
	}
	if err := s.Compact(true); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	before, err := s.Usage(dbPath)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}

	dir := t.TempDir()
	res, err := s.ArchiveTier(RollupMinute, dir, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("ArchiveTier: %v", err)
	}
	if err := s.Compact(true); err != nil {
		t.Fatalf("Compact after archive: %v", err)
	}

	after, err := s.Usage(dbPath)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}

	archive, _ := os.Stat(res.Path)
	t.Logf("archived %d rows", res.Rows)
	t.Logf("database %d -> %d bytes (%.1f%% smaller)", before.DatabaseBytes, after.DatabaseBytes,
		100*(1-float64(after.DatabaseBytes)/float64(before.DatabaseBytes)))
	t.Logf("archive on disk: %d bytes (%.1fx smaller than the rows it holds)",
		archive.Size(), float64(before.DatabaseBytes-after.DatabaseBytes)/float64(archive.Size()))

	if after.DatabaseBytes >= before.DatabaseBytes {
		t.Errorf("database did not shrink: %d -> %d", before.DatabaseBytes, after.DatabaseBytes)
	}
	if after.RollupRows >= before.RollupRows {
		t.Errorf("rollup rows did not fall: %d -> %d", before.RollupRows, after.RollupRows)
	}
}
