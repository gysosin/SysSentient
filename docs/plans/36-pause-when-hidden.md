# 36 — Stop the dashboard working while nobody is looking

| | |
|---|---|
| **Phase** | 5 — Console |
| **Depends on** | 25 |
| **Status** | **done** — a hidden tab holds no socket and runs no timers |

## Why

Chrome flagged the dashboard tab: *"This tab is using extra resources. To
improve your performance, let Chrome make it inactive."* That heuristic is about
what a tab does **in the background**, and measurement showed the dashboard did
everything in the background that it did in the foreground.

Measured on the running instance, Overview page, no interaction:

| | Hidden tab | Visible tab |
|---|---|---|
| DOM mutations / second | **43–76** | 163 |
| Long tasks in 9 s | 6 (one of **379 ms**) | 0 |
| Running animations | 33–43, 3 infinite | 28, 3 infinite |
| Heap over 20 s | — | 47.6 → 79.6 MB sawtooth, 5 GCs |

Nothing in `web/` read `document.visibilityState`. Chrome throttles
`setInterval` in a background tab but **does not throttle WebSocket
`onmessage`** — so a hidden tab kept receiving a frame every two seconds,
JSON-parsing it, allocating fresh `Process[]` and `Filesystem[]` arrays, and
triggering two full-application renders per frame. The heap is not leaking (it
sawtooths, so the collector reclaims it) but it allocates roughly 3 MB of
garbage a second, and with the tab hidden none of that produces anything anyone
sees.

## What changed

- `web/hooks/usePageVisible.ts` — one hook, one source of truth for
  `visibilitychange`. Defaults to visible where `document` is absent, so tests
  and any non-browser runtime keep the behaviour they had.
- `useWebSocket(enabled)` — when the tab is hidden the socket is **closed**, not
  merely ignored: that also stops the server encoding and sending a frame to
  this client at all. Re-enabling reconnects **immediately**, bypassing the
  reconnect backoff, because the operator is looking at the screen again and a
  30-second wait would show them a stale dashboard for no reason.
- Every periodic effect gains `if (!visible) return` and `visible` in its
  dependency list: the 1 s clock tick, the 2 s polling fallback, logs (10 s),
  firing alerts (10 s), hosts (15 s), insight history (30 s), notifications
  (15 s), and the page-level pollers on Alerts (5 s, four requests per tick),
  Hosts, Settings and Devices.
- Because each of those effects already fetches once before starting its
  interval, re-arming on `visible` gives the **catch-up for free**: the moment
  the tab is shown, every poller refreshes once, then resumes. No new code path.

## What deliberately did not change

- Visible behaviour. Same 2-second cadence, same charts, same animations. The
  acceptance test for this shard is that the visible probe reads the same after
  as before.
- CSS and motion animations. Chrome does not paint hidden tabs and pauses
  `requestAnimationFrame`, so they already cost nothing while hidden.

## Acceptance

- `usePageVisible` — 3 tests: initial state, flips both ways, unsubscribes.
- `useWebSocket` — 3 tests: no socket while disabled; disabling closes the
  socket and does not reconnect; re-enabling opens a fresh socket at once.
- Live: with the tab hidden, `/health` `websocketClients` drops by one and the
  DOM-mutation probe reads ~0/s against 43–76 before; showing the tab restores
  LIVE within a few seconds with no gap in the charts.

## Not done in this shard — all since closed

Everything below was left for later when this shard landed. It was all done in
[37-visible-tab-render-cost.md](37-visible-tab-render-cost.md), except the
`scanline`, whose entry here overstated the cost — the correction is inline.

The visible tab still does ~163 DOM mutations a second while idle, all of it
waste with no visible effect. Ranked by measured impact:

1. `now` (a 1 s tick) in the context memo dependencies —
   `useDashboardData.tsx` — one full-tree render per second for a string only
   `feed.detail` reads.
2. Two renders per WebSocket frame (set in `useWebSocket`, merge again in
   `useDashboardData`); `normalizeProcesses` allocates ten objects per frame
   whether or not anything changed.
3. ~28 springs animating CSS `width` — a layout property — re-triggered every
   2 s. `transform: scaleX` would composite.
4. Eight `AnimatedNumber`s each firing React `setState` at frame rate for
   ~75% of every cycle.
5. Seven `Sparkline`s whose `useMemo` is defeated by a fresh array per render.
6. `scanline`: a 7-second infinite gradient sweep on the hero. Corrected
   after checking the CSS -- it already animates `transform`, which the
   compositor handles, so its cost is keeping the tab from reaching idle
   rather than layout or paint work. Removing it is a design call, not a
   performance fix.
7. `SystemChart` unmemoised, ~24 Redux dispatches a second across four charts.
