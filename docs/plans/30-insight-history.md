# 30 — Insight history

| | |
|---|---|
| **Phase** | 4 — Correctness |
| **Depends on** | 24 |
| **Status** | **done** — every analysis is kept, attributed and shown |

## Why

Live proof before the change: **6 analyses in the database, dashboard reporting
"No analysis yet."** Four independent faults stacked up.

1. **`storage.Insight` had no JSON tags**, so the API emitted
   `{"Content":…,"Timestamp":…}` while the client read `content`/`timestamp`.
   Every stored analysis was unreadable.
2. **The loader was unreachable over WebSocket.** `fetchLatestInsight` ran only
   inside the polling fallback, which returns early when connected — so a
   healthy socket meant no insight was ever loaded.
3. **Only one survived.** The server returned ten; the client took `data[0]`
   and discarded the rest.
4. **`/api/insights` accepted nothing** — limit hardcoded to 10, no host, no
   range.

Beyond those: no `host_id`, so a fleet could not attribute an analysis;
`CURRENT_TIMESTAMP` gave second resolution where every other row stores
milliseconds; and **a cache hit was persisted as a new row**, so an unchanging
machine accumulated identical analyses under different timestamps.

## What changed

- `Insight` gains JSON tags, an `id`, `host_id`, `status` and a millisecond
  timestamp. `status` is lifted out of the JSON body so a timeline can filter
  and colour without parsing every row.
- `ListInsights(InsightQuery)` filters by host, status and time window.
  `/api/insights` accepts `limit`, `host`, `status`, `from` and `to`.
- `AnalyzeSystemState` reports whether an answer came from the cache, and
  callers no longer store duplicates.
- The dashboard loads history on its own schedule, independent of transport,
  and the Insights page shows a clickable timeline of every stored analysis.
- The superseded `SaveInsight`/`GetRecentInsights` are removed.

## Acceptance

```
GET /api/insights?limit=5
  keys: ['content', 'host_id', 'id', 'status', 'timestamp']    (lowercase)
  id=18  status=Warning  host=5d3733f8b7af  2026-09-04T13:00:36.139Z
  id=17  status=-        host=-             2026-09-04T12:59:42Z   (pre-migration)

?status=Warning → 1 row     ?limit=0 → 400
```

In the browser, with the WebSocket connected: **18 stored**, all eighteen
listed with relative timestamps, newest selected. Rows written before the
migration keep their content and simply carry no status or host.
