# 07 — Fix SQLite concurrency and reclaim disk

| | |
|---|---|
| **Phase** | 2 — Performance |
| **Depends on** | 01 |
| **Status** | **done** — concurrency, batching, checkpoint, vacuum, and a 15.1% smaller row |

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


## Row payload — measured

Averaging `LENGTH(CAST(col AS BLOB))` per column over a real database:

| Column | Before | After |
|---|---:|---:|
| `processes` | 1027 B | 1003 B |
| `filesystems` | 600 B | 543 B |
| `top_processes` | 250 B | **0 B** |
| `cpu_per_core` | 147 B | 144 B |
| everything else | 225 B | 219 B |
| **total** | **2249 B** | **1909 B** |

**15.1% smaller**, almost entirely from one change: `top_processes` was a
human-readable rendering of `processes` — the identical data, formatted — so it
is now derived on read. Consumers still see it populated; the AI prompt reads
it back out of storage, and an empty value would silently degrade analysis.

The filesystem columns barely moved, and that is worth recording. `free_bytes`
and `used_percent` *look* derivable from `total_bytes` and `used_bytes`, and an
earlier version of this change dropped them on that basis for a headline 19.8%.
It was wrong. On this machine:

| Mount | fstype | free reported | total − used |
|---|---|---:|---:|
| `/` | btrfs | 242,961,608,704 | 244,080,635,904 |
| `/boot` | ext4 | 1,360,130,048 | 1,484,279,808 |
| `/boot/efi` | vfat | 606,896,128 | 606,896,128 |

Reserved blocks mean the derivation holds only where nothing is reserved. The
encoding now drops a derived field **only when it actually matches** the
derivation, making the compaction lossless by construction rather than by
assumption — so it saves on vfat, tmpfs and overlay mounts and nothing on the
root filesystem.

`processes` is now 53% of a row and is not redundant: it is the per-sample
drill-down data the console shows. Reducing it further means storing less
history, not storing the same history more compactly.
