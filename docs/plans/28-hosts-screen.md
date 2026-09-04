# 28 — Hosts as a first-class screen

| | |
|---|---|
| **Phase** | 5 — Console |
| **Depends on** | 27 |
| **Status** | **done** — one meaning, one page |

## Why

"Host" meant three unreconciled things: a `hostname` string on a sample, a row
from `/api/hosts`, and an enrolled agent from `/api/agents`. There was **no
hosts page** and no per-host detail, and the switcher stayed hidden until a
second machine existed — so on every single-node install the concept, and the
path to adding another machine, were both invisible.

## What changed

- A `/hosts` route listing every machine with its live state, label, agent
  version and last-seen, joining the host and agent views **by id**.
- The **host id is shown**, not just the hostname. Two machines can share a
  name — this demo has two called `fedora` — and everything else keys on the id.
- Badges distinguish **this machine**, **local** (reporting without an enrolled
  credential: the all-in-one install collecting from itself, or an agent on the
  shared fleet key) and **revoked**.
- Selecting a host scopes the whole console to it and returns to the overview.
- The switcher is shown from one host, and `/hosts` is in the nav and on the
  number-key shortcuts.

## Acceptance

Live, with two machines that share a hostname:

```
MACHINES                                                    2
● build-server  fedora  THIS MACHINE
  5d3733f8b7af49c7 · dev · last seen 20s ago
○ fedora  LOCAL
  build-server-dem · dev · last seen 2h ago
```

The two are distinguishable because the id is on screen, which is the point.
