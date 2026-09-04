package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"sys-sentient/internal/config"
	"sys-sentient/internal/models"
	"sys-sentient/internal/storage"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// seedAgedSamples writes samples old enough to be past the raw retention
// window, which is the only data Rollup aggregates.
func seedAgedSamples(t *testing.T, store *storage.Store, hostID string, n int, age time.Duration) {
	t.Helper()
	base := time.Now().Add(-age)
	batch := make([]*models.SystemState, 0, n)
	for i := range n {
		batch = append(batch, &models.SystemState{
			HostID:      hostID,
			Hostname:    "seeded",
			Timestamp:   base.Add(time.Duration(i) * time.Second),
			CPUUsage:    float64(10 + i%40),
			MemoryUsed:  uint64(1 << 30),
			MemoryTotal: uint64(8 << 30),
			LoadAvg1:    1.5,
		})
	}
	if _, err := store.SaveBatch(batch); err != nil {
		t.Fatalf("SaveBatch: %v", err)
	}
}

// maintenanceFixture returns a runner over a real store, plus the path so a
// test can count rows without the store needing a method that exists only for
// tests.
func maintenanceFixture(t *testing.T) (*maintenance, *storage.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "maint.db")
	store, err := storage.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			MetricsRetentionHours:  24,
			MinuteRollupDays:       30,
			FiveMinuteRollupDays:   365,
			InsightsRetentionHours: 168,
		},
		Collector: config.CollectorConfig{PollIntervalSeconds: 2},
		Logging:   config.LoggingConfig{Level: "info", Format: "text"},
	}
	runner := newMaintenance(store, config.NewRuntime(cfg), cfg.Database.InsightsRetentionHours, "", quietLogger())
	return runner, store, path
}

func countRows(t *testing.T, path, table string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()

	var n int
	// #nosec G201 -- table is a literal from this test, never external input.
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %q", table)).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestMaintenanceRollsUpBeforePruning(t *testing.T) {
	maint, store, path := maintenanceFixture(t)
	// Older than the 24h raw window, so it is both rollup-eligible and
	// prune-eligible on the same tick. Getting the order wrong destroys it.
	seedAgedSamples(t, store, "host-1", 120, 48*time.Hour)

	if got := countRows(t, path, "metric_rollups"); got != 0 {
		t.Fatalf("rollups before maintenance = %d, want 0", got)
	}

	maint.run(time.Now())

	if got := countRows(t, path, "metric_rollups"); got == 0 {
		t.Fatal("no rollups after maintenance: the aged samples were deleted without being aggregated")
	}
	// ...and the raw rows they came from are now gone.
	if got := countRows(t, path, "metrics"); got != 0 {
		t.Errorf("raw metrics after maintenance = %d, want 0 (past the retention window)", got)
	}
}

func TestMaintenancePrunesExpiredJoinTokens(t *testing.T) {
	maint, store, path := maintenanceFixture(t)
	now := time.Now()
	if err := store.CreateJoinToken("live", "hash-live", "live", "admin", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	if err := store.CreateJoinToken("dead", "hash-dead", "dead", "admin", now.Add(-2*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}

	maint.run(now)

	// Expired invitations accumulated forever: nothing called
	// PruneExpiredJoinTokens outside its own test.
	if got := countRows(t, path, "join_tokens"); got != 1 {
		t.Errorf("join_tokens after maintenance = %d, want 1 (the unexpired one)", got)
	}
}

func TestMaintenanceCompactsOnScheduleOnly(t *testing.T) {
	maint, store, _ := maintenanceFixture(t)
	seedAgedSamples(t, store, "host-1", 10, 48*time.Hour)

	// A full VACUUM takes an exclusive lock and rewrites the file, so it must
	// not run on every tick.
	for range vacuumEveryNTicks - 1 {
		maint.run(time.Now())
	}
	if maint.vacuumsRun != 0 {
		t.Errorf("vacuums after %d ticks = %d, want 0", vacuumEveryNTicks-1, maint.vacuumsRun)
	}

	maint.run(time.Now())
	if maint.vacuumsRun != 1 {
		t.Errorf("vacuums after %d ticks = %d, want 1", vacuumEveryNTicks, maint.vacuumsRun)
	}
	_ = store
}

func TestMaintenanceSurvivesAnEmptyDatabase(t *testing.T) {
	maint, _, _ := maintenanceFixture(t)
	// Must not panic or wedge on a fresh install with nothing to do.
	maint.run(time.Now())
	maint.run(time.Now())
}

// TestServerModeRunsMaintenance is the regression guard for this shard.
//
// Server mode used to run its own shorter maintenance list that omitted the
// rollup entirely, so a fleet server hard-deleted raw samples at the retention
// cutoff without ever aggregating them. Its own doc comment claimed retention
// still ran. Nothing caught it because nothing exercised the loop.
func TestServerModeRunsMaintenance(t *testing.T) {
	maint, store, path := maintenanceFixture(t)
	maint.interval = 20 * time.Millisecond
	seedAgedSamples(t, store, "fleet-host", 120, 48*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Stopped through the server error channel rather than the context, so the
	// loop returns without entering the HTTP shutdown path this test has no
	// server for.
	serverErr := make(chan error)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runServerOnly(ctx, quietLogger(), nil, maint, serverErr)
	}()

	deadline := time.After(3 * time.Second)
	for countRows(t, path, "metric_rollups") == 0 {
		select {
		case <-deadline:
			t.Fatal("server mode produced no rollups: aged samples are being deleted without being aggregated")
		case <-time.After(20 * time.Millisecond):
		}
	}

	close(serverErr)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runServerOnly did not stop when the server exited")
	}
}
