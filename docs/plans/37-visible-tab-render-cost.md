# 37 — Stop re-rendering the console once a second

| | |
|---|---|
| **Phase** | 5 — Console |
| **Depends on** | 36 |
| **Status** | **done** — the clock no longer redraws the dashboard |

## Why

Shard 36 stopped the dashboard working while its tab was hidden. Visible, it
still did **163 DOM mutations a second** while idle, and allocated ~3 MB/s of
garbage. None of that produced anything a person could see.

The cause was structural. A 1-second clock tick drives the "updated Ns ago"
readout, and `now` sat in the dependency list of the context value's `useMemo`.
So every second a brand-new context object was published and React re-rendered
**all eleven consumers** — including the four charts, eight tiles and seven
sparklines on the Overview — to update a string displayed in two small places.

Measured with a deterministic test (`useDashboardData.rerender.test.tsx`):
a consumer reading only `hosts`, over ten simulated seconds with no data
arriving, was re-rendered **10 times**. One per tick.

## What changed

- **The feed moved to its own context.** `useFeed()` subscribes to the badge;
  `useDashboard()` no longer carries it, and `now` is out of the main memo's
  dependencies. Components that show the age — the header chip, the last-sync
  clock, the degraded banner, the hero eyebrow, the "last sample" note — are now
  small leaves that re-render alone.
- **Overview is wrapped in `StaleDimmer`**, which takes the page as `children`.
  It re-renders every second to toggle one class, but `children` is the same
  element object each time, so React skips the subtree beneath it.
- **`toggleFreeze` is stable.** Its dependencies changed on every frame, so it
  was a new function twice a second and re-published the context by itself.
- **`AnimatedNumber` renders its MotionValue directly.** It previously mirrored
  every spring frame into React state, so each of eight numbers drove its own
  render for roughly three quarters of every two-second cycle. The suffix is
  folded into the transform so the element keeps exactly one text node —
  splitting it breaks any lookup that reads an element's own text.
- **`SystemChart` takes `onZoom` as a prop and is memoised.** It read the
  dashboard context, and a context change re-renders a memoised component
  regardless of props, so all four charts redrew on every publish.
- **The seven sparkline series are memoised** on `metricsHistory`. They were
  built inline in JSX, so each render produced seven fresh 32-element arrays and
  `Sparkline`'s own `useMemo` — keyed on array identity — recomputed its
  geometry every time.

## Acceptance

`useDashboardData.rerender.test.tsx`, deterministic and in CI:

| | Renders in 10 simulated seconds |
|---|---|
| Before | **10** |
| After | **0** |

Verified as a genuine guard by restoring `now` to the dependency list: the test
fails with `expected 10 to be less than or equal to 2`, and passes again once
reverted. A companion test asserts the feed consumer **still** updates every
second, so the readout cannot be silently broken by this optimisation.

## Not verified live

The browser-side before/after (163 mutations a second) could not be re-measured:
Chrome was backgrounded for the whole session, and after shard 36 a hidden tab
deliberately does no work, so there was no visible tab to probe. The evidence
above is the deterministic test, not a browser measurement.

## Not done, deliberately

- **~28 springs animating CSS `width`** — a layout property — on the tile fills,
  per-core bars and `Meter`. `transform: scaleX` composites instead, but it
  distorts the rounded ends of a `rounded-full` bar, so it is a visual change
  and needs a design decision rather than a silent swap.
- **`scanline`** — a 7-second infinite gradient sweep on the Overview hero.
  I checked the CSS after writing the earlier note: it already animates
  `transform`, so the compositor handles it and there is no layout or paint
  cost. What remains is that the tab never reaches a fully idle state.
  Removing it is a design call, not a performance fix.
- **The width springs are now done too** — see below. Nothing on the original
  list remains.

## The bars: slid, not stretched

~28 bars — one per CPU core, plus every tile fill and meter — animated CSS
`width`, a layout property, twice a second.

`transform: scaleX` would composite but squashes the rounded end cap of a
`rounded-full` fill, so it was not a silent swap to make. Instead each fill is
now **full width with its rounded ends intact and translated left** by the
remainder; the track's `overflow-hidden` clips it. The rounded cap still sits
exactly at the fill point, and `transform` stays on the compositor.

A 45% meter is `translateX(-55%)`. That inversion is the easy thing to get
backwards — it would render every bar as its own complement while breaking
nothing else — so `components/ui/meter.test.tsx` pins the geometry at 0%, 45%
and 100%, the clamping either side, and the `aria-valuenow` that a visual
change must not quietly drop.

**Two width animations remain, deliberately:** the memory panel's split bar is
two segments side by side in flow, each sized as a share of one track, so
neither can be translated without moving the other. Two elements on one panel,
against the ~28 converted. A test asserts exactly those two survive.

**Not verified by eye.** Chrome was backgrounded for the entire session and a
hidden tab renders the empty state, so there were no bars to look at. The
geometry is pinned by test and equivalent by construction, but nobody has seen
it.
