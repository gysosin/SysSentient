# 20 — Per-widget drill-down

| | |
|---|---|
| **Phase** | 4 — Console |
| **Depends on** | 06, 13 |
| **Status** | not started |

## Why

Clicking a metric tile does nothing. The chart-expand dialog added earlier only
enlarges the same series; it does not answer "what is happening, and where".
Tiered retention (shard 06) is a prerequisite for a useful time-range selector
inside the drill-down.

## Scope

Clicking any tile opens a full analysis view:

- **CPU** — per-core history, top consumers, load-average correlation.
- **Memory** — composition over time, top consumers, swap pressure.
- **Disk** — per-filesystem history, per-device IO, inode pressure.
- **Network** — per-interface throughput.

Each with a time-range selector backed by the retention tiers. Built on the
existing `Dialog` primitive.

## Acceptance criteria

- Every tile is clickable and keyboard-accessible.
- Each view answers *where* the load is, not just how much.
- The range selector reflects data that actually exists — no empty ranges.

## Verification

```bash
cd web && npm test
```
Plus a manual pass under `stress-ng` load confirming the CPU drill-down
identifies the responsible process.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
