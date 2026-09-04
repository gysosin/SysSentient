package main

import (
	"log/slog"
	"time"

	"sys-sentient/internal/config"
	"sys-sentient/internal/storage"
)

// vacuumEveryNTicks spaces out the full VACUUM. It takes an exclusive lock and
// rewrites the whole file, so it runs daily rather than on every tick; the WAL
// checkpoint beside it is cheap and runs every time, because under continuous
// writes the WAL otherwise grows past the database it belongs to.
const vacuumEveryNTicks = 24

// maintenanceInterval is how often the upkeep above runs. Shared by every mode
// so the vacuum schedule above means the same thing in all of them.
const maintenanceInterval = time.Hour

// maintenance is the periodic database upkeep every mode needs.
//
// It lives in one place because it did not used to. Server mode ran its own
// shorter list that omitted the rollup entirely, so a fleet server hard-deleted
// raw samples at the retention cutoff without ever aggregating them: the year
// of history the tiers promise was destroyed on the deployment that needed it
// most, silently, and its own doc comment claimed otherwise. Two copies of a
// sequence whose *order* is load-bearing is not a thing to keep.
type maintenance struct {
	store   *storage.Store
	runtime *config.Runtime
	// insightsRetentionHours also governs alert events, which have no
	// retention setting of their own.
	insightsRetentionHours int
	logger                 *slog.Logger

	// interval is how often run should be called. Carried here so the loops
	// that drive it, and the tests that exercise those loops, agree on one
	// value instead of each holding a literal.
	interval time.Duration

	ticks int
	// vacuumsRun counts full compactions, so a test can assert the schedule
	// without waiting a day for one.
	vacuumsRun int
}

func newMaintenance(
	store *storage.Store,
	runtime *config.Runtime,
	insightsRetentionHours int,
	logger *slog.Logger,
) *maintenance {
	return &maintenance{
		store:                  store,
		runtime:                runtime,
		insightsRetentionHours: insightsRetentionHours,
		logger:                 logger,
		interval:               maintenanceInterval,
	}
}

// runOnStart performs the first pass immediately.
//
// The hourly ticker alone means a restarted daemon does no upkeep for an hour,
// and a fresh install has empty rollup tiers until one fires -- so /api/export
// and any historical query return nothing all that time, which reads as a
// broken feature rather than a young one.
func (m *maintenance) runOnStart(now time.Time) {
	m.logger.Debug("running startup maintenance")
	m.run(now)
}

// run performs one maintenance pass.
//
// Every step is independent: a failure is logged and the rest still run, so one
// wedged table cannot stop the database being pruned and compacted.
func (m *maintenance) run(now time.Time) {
	if m.store == nil {
		return
	}

	rawHours, minuteDays, fiveMinuteDays := m.runtime.Retention()
	policy := storage.RetentionPolicy{
		RawHours:       rawHours,
		MinuteDays:     minuteDays,
		FiveMinuteDays: fiveMinuteDays,
	}

	// Roll up before pruning, never after: PruneTiers deletes the raw samples
	// the rollup reads, so reversing these two loses the history permanently
	// rather than aggregating it. PruneTiers is skipped when the rollup fails
	// for the same reason.
	if err := m.store.Rollup(policy, now); err != nil {
		m.logger.Error("error rolling up metrics", "error", err)
	} else if err := m.store.PruneTiers(policy, now); err != nil {
		m.logger.Error("error pruning metric tiers", "error", err)
	}

	if err := m.store.PruneOldInsights(m.insightsRetentionHours); err != nil {
		m.logger.Error("error pruning old insights", "error", err)
	}
	if err := m.store.PruneOldAlertEvents(m.insightsRetentionHours); err != nil {
		m.logger.Error("error pruning old alert events", "error", err)
	}
	if _, err := m.store.PruneExpiredSessions(now); err != nil {
		m.logger.Error("error pruning expired sessions", "error", err)
	}
	// Unredeemed invitations expire but were never deleted, so they
	// accumulated for the life of the install.
	if _, err := m.store.PruneExpiredJoinTokens(now); err != nil {
		m.logger.Error("error pruning expired join tokens", "error", err)
	}

	// Reclaim what the deletes above freed.
	m.ticks++
	vacuum := m.ticks%vacuumEveryNTicks == 0
	if err := m.store.Compact(vacuum); err != nil {
		m.logger.Warn("error compacting database", "error", err)
		return
	}
	if vacuum {
		m.vacuumsRun++
	}
}
