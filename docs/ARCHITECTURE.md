# Architecture

## Shape

One binary, three modes.

    all-in-one   collects locally, stores, serves the dashboard and API
    server       stores and serves; agents push to it
    agent        collects and pushes; no database, no dashboard

A single binary with modes rather than separate executables: the separation is
logical, and every extra binary doubles an already awkward build matrix.

## Components

| Package | Responsibility |
|---|---|
| `cmd/daemon` | Entry point, the collect loop, graceful shutdown |
| `internal/collector` | Metric collection via gopsutil, plus a direct `/proc` reader on Linux |
| `internal/storage` | SQLite persistence, migrations, tiered rollups, backup |
| `internal/server` | HTTP routes, auth, WebSocket hub, ingest |
| `internal/alerting` | Rule evaluation, the pending→firing→resolved machine, notification |
| `internal/auth` | argon2id hashing, sessions, roles, the first-run token |
| `internal/ai` | Gemini analysis, circuit breaker, spend cap |
| `internal/pii` | Redaction applied before anything reaches the model |
| `internal/agent` | Push client and its on-disk spool |
| `internal/logs` | Per-platform log collection |
| `internal/hostid` | Stable machine identity |
| `web/` | React 19 dashboard, embedded into the binary at build time |

## Data flow

    collector ─▶ storage ─▶ ┌─ HTTP API ─▶ dashboard
                            ├─ WebSocket ─▶ live updates
                            ├─ alerting ─▶ webhook / Slack
                            └─ Prometheus /metrics

In agent mode the collector pushes to a remote `/api/ingest` instead, buffering
to a local spool when the network is down.

## Decisions worth knowing

**The SQLite driver is pure Go.** `modernc.org/sqlite` rather than
`mattn/go-sqlite3`, which needed cgo and made `CGO_ENABLED=0` impossible —
no static binary, and no Windows, macOS or arm64 target without a C toolchain
per platform.

**Timestamps go through one explicit layout.** `database/sql` leaves parameter
encoding to the driver, and binding a raw `time.Time` produced a format
SQLite's own date functions could not parse.

**The dashboard is embedded.** Serving it from a path relative to the working
directory made the binary non-relocatable, which is why the systemd unit used
to pin `WorkingDirectory` and why no package could ship it.

**Retention is tiered, not a cut-off.** Full resolution for a day, per-minute
for a month, per-five-minute for a year. Keeping raw samples for a year would
be tens of gigabytes per host.

**Severity has hysteresis.** Thresholds widen once crossed, so a machine
sitting at 89–91°C against a 90°C limit does not rewrite the page every two
seconds.

## Platform support

Metric collection is cross-platform via gopsutil. Log collection and machine
identity have per-platform implementations:

| | Logs | Machine identity |
|---|---|---|
| Linux | journalctl, dmesg, `/var/log/syslog` | `/etc/machine-id` |
| Windows | System and Application event logs (`wevtutil`) | registry `MachineGuid` |
| macOS | unified log (`log show`) | `IOPlatformUUID` |

Every source fails soft, so a missing or unreadable one costs the others
nothing. Service integration is Linux-only so far.
