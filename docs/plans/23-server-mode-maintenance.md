# 23 — Make server mode a first-class citizen

| | |
|---|---|
| **Phase** | 4 — Correctness |
| **Depends on** | 06, 07 |
| **Status** | **done** — one maintenance path for every mode |

## Why

`runServerOnly` ran its own maintenance list: `PruneOldMetrics`,
`PruneOldInsights`, `PruneOldAlertEvents`, `PruneExpiredSessions`. It never
called `Rollup`, `PruneTiers` or `Compact` — verified, zero call sites.

The all-in-one loop got this right and even documented why the order matters:
"Roll up before pruning, never after: PruneTiers deletes the raw samples the
rollup reads." Server mode skipped the rollup and went straight to the delete.

So on the fleet deployment — the one with the most data, and the only one where
losing it matters — every consequence was silent:

- Raw metrics hard-`DELETE`d at the retention cutoff. **Everything older gone.**
- `metric_rollups` empty forever, so the 30-day and 1-year tiers did not exist
  and `/api/export` returned nothing.
- WAL never checkpointed, database never vacuumed: the file only grew.

`runServerOnly`'s own doc comment said *"Retention still runs, because the
server owns the fleet's data."* It did not.

Two copies of a sequence whose *order* is load-bearing is not a thing to keep.

## Scope

- Extract the maintenance body into `cmd/daemon/maintenance.go`; both modes call
  the same `maint.run(now)`.
- Honour `alerting.enabled` on the ingest path. Fixed at construction — the
  evaluator is only built when alerting is on — so every caller inherits it
  instead of each remembering to check.
- Build the runtime before the mode branch and call `SetRuntime` in both, so
  `GET`/`PATCH /api/settings` stop returning 404 in server mode.
- Call `PruneExpiredJoinTokens`, which nothing outside its own test called.
- Add `host_id` to `alert_events` alongside `hostname`, with a migration for
  existing databases.

## Acceptance

- `TestServerModeRunsMaintenance` drives the real `runServerOnly` loop and fails
  if it stops rolling up. Verified by reverting the fix: the test fails, and
  passes again once restored.
- An older database gains `alert_events.host_id` without losing existing rows.
- Ingest with alerting disabled records no alert events.
