# 07 — Fix SQLite concurrency and reclaim disk

| | |
|---|---|
| **Phase** | 2 — Performance |
| **Depends on** | 01 |
| **Status** | not started |

## Why

`internal/storage/sqlite.go:23` sets `SetMaxOpenConns(1)`, so WAL is enabled and
then neutered — every read serialises behind every write. `handleIngest` makes
one autocommit `INSERT` per sample with no transaction, through that single
connection. There is no `VACUUM` anywhere, so a database that spikes stays
large forever.

## Scope

- Raise `SetMaxOpenConns`; WAL supports concurrent readers with one writer.
- Wrap ingest batches in a single transaction.
- Periodic `wal_checkpoint(TRUNCATE)` — the WAL was measured larger than the
  database itself (4.0 MB vs 1.8 MB).
- Add `VACUUM` on the maintenance tick.
- Stop writing full process and filesystem JSON on every row.

## Acceptance criteria

- **Under 20 MB per host per day** steady state, against today's ~171 MiB.
- Dashboard reads do not block during ingest — verify with a concurrent load
  test.
- The database file shrinks after a retention pass.

## Verification

```bash
du -h sys-sentient.db*
sqlite3 sys-sentient.db "select count(*) from metrics;"
```
Measure before and after a 24h run.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
