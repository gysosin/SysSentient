# 26 — Export from the UI

| | |
|---|---|
| **Phase** | 5 — Console |
| **Depends on** | 24, 25 |
| **Status** | **done** — the current window, downloadable |

## Why

`/api/export` has been complete server-side since the backup shard — format,
resolution, window, host, row cap — and **no part of the UI called it**. Getting
data out meant constructing the URL by hand.

## What changed

- `exportMetrics()` downloads a window as CSV or JSON, reading the whole body
  before creating the link so a failed stream cannot save a truncated file that
  looks complete, and preferring the server's own filename.
- An export control beside the charts, so what it downloads is the window
  visible above it — the selected range, the selected host, and the tier that
  answered it.
- `fetchWithTimeout` takes a per-call timeout: an export of a long window
  legitimately outlives the 8-second default without that default rising for
  every other request.

## Acceptance

Driven through the running dashboard:

```
export button rendered: true      range selected: 1h
GET /api/export?format=csv&resolution=raw&since=… → 200
Content-Disposition: attachment; filename="sys-sentient_raw_20260904.csv"
1778 rows
timestamp,host_id,hostname,cpu_usage,memory_used,memory_total,swap_use…
2026-09-04T12:35:29Z,5d3733f8b7af49c706e…
```
