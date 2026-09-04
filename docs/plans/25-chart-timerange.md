# 25 — Drill-down on every graph, and a time picker

| | |
|---|---|
| **Phase** | 5 — Console |
| **Depends on** | 24 |
| **Status** | **done** — one window, respected everywhere |

## Why

The daemon retains up to a year. The console showed **two to four minutes** of
it: the client seeded 50 points and capped at 120, charts had no brush, zoom or
range picker, and `SystemChart`'s "expand" dialog re-rendered the same data
larger.

## What changed

- **A range picker in the header** — Live / 15m / 1h / 6h / 24h / 7d / 30d —
  that the whole dashboard respects. `live` keeps the streaming tail; anything
  else is a bounded query.
- **Drag-to-zoom on any chart**, producing a `custom` window. The chart's own
  axis is not rescaled in isolation: the range lives in one place so every
  chart and the process list move together, because reading a spike means
  seeing what else the machine was doing at that moment.
- **The tier is shown** — "every sample", "1-minute averages", "5-minute
  averages" — so a chart never implies raw samples while drawing averages.

## Two bugs found by using it

**A young install rendered every window empty.** Rollup tiers only hold data old
enough to have been aggregated, so on a fresh instance a 24-hour selection
returned nothing beside a database full of samples. The server now falls back to
raw over the part of the window raw can answer, **and reports that narrower
window back** rather than labelling an hour of samples as a day.

**The row cap truncated instead of decimating.** A six-hour request returned the
*oldest* 2,000 samples — thirty-three minutes of data, still labelled six hours:

```
requested 6h window, got 2000 points
  data actually spans: 0:33:19   => TRUNCATED
```

`QueryRange` now decimates evenly across the window with a stride computed in
the query itself, one round trip.

## Acceptance

```
1h  requested -> raw   1849 points spanning 1:00:00
6h  requested -> raw   1985 points spanning 5:59:58
24h requested -> raw   1963 points spanning 7:01:47   (all this instance holds)
```

Each window now covers what it claims.
