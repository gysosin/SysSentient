# 31 — Notification centre and rule control

| | |
|---|---|
| **Phase** | 5 — Console |
| **Depends on** | 23 |
| **Status** | **done** — a bell, mutable rules, and no more flapping |

## Why

**There was no notification surface at all.** A repo-wide search for
`toast|notification|bell|unread` found only a `BellOff` icon. Alerts lived on
their own page and as a count on a nav link, so anything happening while you
looked elsewhere was invisible until you went looking for it.

**Rules were hardcoded and read-only.** `Rule.Enabled` was honoured by the
evaluator but nothing could ever set it false — the endpoint had no PATCH, and
`ReplaceRules` existed with no production caller. A threshold could not be
tuned without editing the source and rebuilding.

**Alerts flapped.** `For` delays escalation only; resolution was instantaneous
on a single non-breaching sample, so a host oscillating at 89.9/90.1 against a
`> 90` rule fired and resolved on every poll, notifying each time.

## What changed

- **A bell with unread state and a panel**, plus per-severity colouring. Unread
  is a local high-water mark, so it is per-viewer and survives a reload.
- **Rules are editable**: threshold, `for`, enable/disable, and **mute-until**.
  Stored server-side as *overrides* — only the differences, so an install that
  never touches a rule follows the defaults as they improve rather than being
  pinned to whatever they were on first start. Reset restores the built-in.
- **Mute suppresses notification, not evaluation.** A muted alert still
  evaluates and still shows on the dashboard; it just stops paging anyone.
  Suppressing evaluation would hide the problem, which is not what mute means.
- **Mutes are bounded** at 30 days. A silence with no end is a rule quietly
  disabled forever, discovered months later during an incident it should have
  caught.
- **Resolve hysteresis**, applied only to *firing* alerts — a pending alert has
  notified nobody, so holding it back serves no purpose. Caught by an existing
  test after the first version applied it to both.
- Overrides reload at startup, so a restart does not silently un-mute
  everything somebody deliberately silenced.

## Acceptance

```
PATCH /api/alerts/rules/swap-high {"mute_hours":8}
  muted_until: 2026-09-04T21:51:33.243Z   overridden: true
PATCH /api/alerts/rules/cpu-high  {"threshold":75}   → 75

— restart —
  cpu-high    threshold=75   muted_until=-
  swap-high   threshold=75   muted_until=2026-09-04T21:51:33.243Z

DELETE /api/alerts/rules/cpu-high → threshold=90, overridden=false
```

`TestAlertDoesNotFlapAroundTheThreshold` oscillates a host across the threshold
twenty times and asserts zero transitions, then that a sustained recovery still
resolves.
