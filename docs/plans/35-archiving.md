# 35 — Archiving, so the disk does not fill

| | |
|---|---|
| **Phase** | 7 — Distribution |
| **Depends on** | 23, 24 |
| **Status** | **done** — 96.4% smaller, and restorable |

## Why

Retention deletes. That is the whole of what the tiers did: raw for a day,
one-minute for a month, five-minute for a year, and then gone. An operator who
sets a year of retention is choosing what the dashboard can query — not
consenting to lose everything older.

And a database that only grows is the disk-fill problem that prompted this.

## What changed

- `ArchiveTier` writes rollup rows older than a cutoff to a gzipped JSON-lines
  file and removes them from the database. **Written and fsynced before
  anything is deleted**: a crash between the two leaves a duplicate archive,
  which is recoverable; the other order loses data.
- Written to a temporary file and renamed into place, so a reader never sees a
  partial archive and a crash mid-write leaves nothing to mistake for a
  complete one. Mode `0600` — it holds the same metrics the database does.
- **JSON lines, not one array**: an archive can be read with `zcat` and `grep`,
  and restored without holding all of it in memory.
- `sys-sentient --restore-archive <file>` brings it back. Idempotent, because
  the rollup table's unique constraint replaces rather than duplicates — so a
  restore run twice, or overlapping an existing range, is safe.
- Archiving runs **before** tier pruning in the same maintenance pass, for the
  same reason the rollup runs before the raw prune: pruning deletes the rows
  the archive would keep.
- A failure is logged and pruning still runs. An unwritable archive directory
  must not stop retention, or the disk fills with exactly the data archiving
  was configured to move off it.
- `Store.Usage` reports database and WAL size, row counts and bytes-per-row, so
  the cost of a retention change can be seen before it is applied.

Off by default (`database.archive_path: ""`), which keeps the historical
behaviour for anyone who wants retention alone.

## Acceptance

`TestArchivingActuallyReclaimsDisk` seeds sixty days of samples, rolls them up,
archives and vacuums:

```
archived 1000 rows
database 8253440 -> 299008 bytes (96.4% smaller)
archive on disk: 3725 bytes (2135.4x smaller than the rows it holds)
```

The test asserts the **file** shrinks, not merely that rows moved — a test that
only counted rows would pass while the disk stayed full, which is the failure
being fixed.
