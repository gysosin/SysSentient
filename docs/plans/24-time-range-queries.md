# 24 — Time-range query engine

| | |
|---|---|
| **Phase** | 4 — Correctness |
| **Depends on** | 23 |
| **Status** | **done** — bounded windows, tier selection, honest errors |

## Why

The daemon retains a year of history in the 1-minute and 5-minute tiers. The
dashboard could show roughly **two to four minutes** of it, because no endpoint
accepted a time range:

- `GET /api/metrics` took only `host` and `limit`.
- `GetRollups` was the only time-bounded read in the store, and it was
  half-open — `since` with no upper bound, so "last Tuesday 14:00–15:00" was
  unaskable.
- `/api/export` had no `until`, and on `resolution=raw` it **ignored `since`
  entirely**, returning the newest N samples. Asking for last Tuesday quietly
  gave you today.
- An invalid `limit` silently became 50 rather than returning 400.

## Scope

- `storage.Range`, `Store.QueryRange` and `Store.GetRollupsRange` — closed
  windows, ascending, host-scoped, with a row cap.
- `ResolveResolution` picks the tier from the window's **age and width**. Age
  matters first: a one-hour window from three days ago is short, but raw
  samples that old have been pruned, so answering from raw returns nothing.
- `GET /api/metrics` accepts `from`, `to`, `resolution=auto|raw|1m|5m` and a
  validated `limit`, and echoes the resolution it chose. Without a window it
  returns the historical bare array, so existing clients are untouched.
- `/api/export` honours the window on every path and accepts `until`.
- The half-open `GetRollups` is gone rather than left beside its replacement.
- Maintenance runs once at startup, so a restart does not leave the database
  untended for an hour and a fresh install has queryable tiers as soon as data
  ages in.

## Acceptance

Verified against a live instance:

```
GET /api/metrics?from=…&to=…   resolution: raw | count: 1189, bounded exactly
GET /api/export?resolution=raw&since=…&until=…   120 rows for a 2-minute window
from=yesterday → 400   limit=0 → 400   resolution=10m → 400
```

## Notes

`+05:30` in an RFC3339 offset decodes to a space in an unencoded query string,
so a correct timestamp typed into curl arrived as `…T17:30:57 05:30` and failed
to parse. The parser restores the sign; a test covers it.

Three copies of the metric scan loop were collapsed into one shared scanner
while adding the fourth reader.
