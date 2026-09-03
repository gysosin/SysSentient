# 06 — Keep the records — tiered retention

| | |
|---|---|
| **Phase** | 2 — Performance |
| **Depends on** | 01 |
| **Status** | not started |

## Why

The product does not currently keep "all the records".
`database.metrics_retention_hours` defaults to **24** and `PruneOldMetrics`
hard-`DELETE`s everything older. History beyond one day is gone.

Measured: 41,553 rows for one host over 24h — **4.3 KB per row**, ~171 MiB per
host per day — because every row carries JSON for `cpu_per_core`, ten
processes and every filesystem.

## Scope

- Tiered retention: raw for 24h → per-minute rollups for 30d → per-5-minute
  for 1y. Every tier configurable.
- A rollup job that aggregates and then deletes the raw rows it replaced.
- Charts query the appropriate tier for the requested range — this is also what
  unblocks the 15m/1h/6h/24h range selector the UI still lacks.

## Acceptance criteria

- A year of history is queryable.
- Storage stays bounded and predictable; document the bytes-per-host-per-day
  figure for each tier.
- No gap or double-count at a tier boundary — covered by a test.

## Verification

```bash
GOTOOLCHAIN=auto go test ./internal/storage/... -race -run Rollup
```
Plus: seed a week of synthetic samples, run the rollup, and confirm row counts
and averages per tier.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
