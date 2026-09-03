# SysSentient

Self-hosted Linux system monitor with alerting, a live dashboard, and optional
AI-assisted analysis via Google Gemini.

- **Metrics** — CPU (total and per-core), memory, swap, disk I/O, **filesystem
  capacity**, network, load averages, temperature, uptime, and top processes by
  *current* CPU usage.
- **Alerting** — threshold rules with a required duration, so transient spikes
  do not page you. Webhook and Slack notifications, acknowledgement, and history.
- **Dashboard** — Overview, Processes, Logs, AI Insights, Alerts and Settings.
- **Integrations** — Prometheus `/metrics`, JSON logs, a `/health` endpoint that
  reports collector liveness rather than just "the process is up".

> **Status:** pre-1.0. Read [SECURITY.md](SECURITY.md) before exposing it
> beyond localhost; the daemon serves plain HTTP, so terminate TLS in front of
> it.

## Architecture

- **Daemon (`sys-daemon`)** — collects metrics, stores them in SQLite, evaluates
  alert rules, and serves the JSON API, WebSocket stream and dashboard.
- **Dashboard (`web/`)** — React 19 + Vite + Tailwind v4 single-page app.

## Quick start

```bash
make build      # dashboard + daemon
./sys-daemon    # dashboard on http://localhost:8080
```

`make help` lists every target. Without a Gemini API key the daemon runs fine
with AI analysis disabled and nothing leaves the machine.

### Container

```bash
docker compose up --build
```

Or directly:

```bash
docker build -t sys-sentient .
docker run --rm -p 127.0.0.1:8080:8080 \
  -v sys-sentient-data:/var/lib/sys-sentient \
  sys-sentient
```

## Configuration

Copy [`config.yaml.example`](config.yaml.example) to `config.yaml`, or use
environment variables — every key maps to `SYS_SENTIENT_` + the path in caps
with dots as underscores (`alerting.webhook_url` →
`SYS_SENTIENT_ALERTING_WEBHOOK_URL`).

| Key | Default | Description |
|-----|---------|-------------|
| `server.port` | 8080 | Web server port |
| `server.api_key` | – | Machine token for scripts (`X-API-Key`). Browser users sign in. See [SECURITY.md](SECURITY.md) |
| `server.insecure` | false | Disables authentication entirely. Warned on every start |
| `auth.session_idle_hours` | 24 | Sign out after this much inactivity |
| `auth.session_max_days` | 30 | Absolute session lifetime |
| `auth.login_rate_per_minute` | 5 | Password attempts per client IP |
| `server.allowed_origins` | localhost:8080, :3000, :5173 | CORS/WebSocket origin allowlist |
| `logging.level` | info | `debug` adds a line per sample |
| `logging.format` | text | `json` for log aggregators |
| `collector.poll_interval_seconds` | 2 | Sampling interval |
| `collector.top_processes` | 10 | Processes recorded per sample |
| `database.metrics_retention_hours` | 24 | Metrics retention |
| `database.insights_retention_hours` | 168 | AI insight and alert history retention |
| `alerting.enabled` | true | Evaluate alert rules |
| `alerting.webhook_url` | – | Receives raw alert JSON per transition |
| `alerting.slack_webhook_url` | – | Slack incoming webhook |
| `gemini.api_key` | – | AI is disabled entirely when empty |
| `gemini.model_name` | gemini-2.5-flash-lite | Model |
| `gemini.max_daily_cost` | 1.0 | Hard USD cap per UTC day; 0 disables |
| `privacy.mask_{ips,emails,usernames}` | true | Redaction before anything reaches Gemini |

## Alerting

Six rules ship enabled by default, covering sustained CPU, memory exhaustion,
swap pressure, filesystem capacity, load average and temperature. Each requires
its condition to hold for a duration before firing, which is what stops a build
spike from paging you.

Set `alerting.webhook_url` or `alerting.slack_webhook_url` to be notified. With
neither set, alerts still appear in the dashboard and the daemon warns at start-up
that nobody will be told.

Rules are currently built in; editing them from the browser is not implemented yet.

## API

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /health` | public | Status, version, database and collector liveness |
| `GET /metrics` | public | Prometheus exposition |
| `GET /api/metrics` | key | Recent samples |
| `GET /api/insights` | key | Recent AI insights |
| `GET /api/logs` | key | Recent system logs (rate limited) |
| `POST /api/analyze` | key | Trigger AI analysis (rate limited — costs money) |
| `GET /api/alerts` | key | Pending and firing alerts |
| `GET /api/alerts/rules` | key | Configured rules |
| `GET /api/alerts/history` | key | Recent transitions |
| `POST /api/alerts/{ruleID}/acknowledge` | key | Silence an active alert |
| `GET /ws/metrics` | key | Live metric stream |
| `GET /api/export` | key | Retained history as CSV or JSON (rate limited) |

## Backup

Do not copy the database file while the daemon is running. SQLite keeps the
database and its write-ahead log as two files that must agree, and a copy taken
between them is corrupt — silently, until you try to restore it.

```bash
sys-daemon --backup /var/backups/sys-sentient-$(date +%F).db
```

That works against a running daemon, writes a self-contained file needing no
`-wal` alongside it, restricts it to `0600` because it contains password hashes
and session tokens, and verifies it with `PRAGMA integrity_check` before
reporting success.

History is also exportable:

```bash
curl -o metrics.csv 'http://localhost:8080/api/export?format=csv&resolution=1m&since=2026-01-01T00:00:00Z'
```

`resolution` selects the tier: `raw`, `1m` or `5m`.

## Privacy

Nothing leaves the machine unless `gemini.api_key` is set. There is no
telemetry, analytics or phone-home. When AI analysis is enabled, the current
sample and recent logs are sent to Google Gemini with IPv4/IPv6 addresses,
e-mail addresses and home-directory usernames redacted first.

**AI-suggested commands are generated by a language model and are not validated
by the daemon.** The dashboard only copies them to the clipboard; it never runs
them. Read them before you do.

## Installation

Every release ships static binaries and packages for linux, windows and darwin
on amd64 and arm64. The dashboard is embedded in the binary, so there is
nothing else to copy and it runs from any directory.

**Debian / Ubuntu**

```bash
sudo dpkg -i sys-sentient_<version>_linux_amd64.deb
```

**Fedora / RHEL**

```bash
sudo rpm -i sys-sentient_<version>_linux_amd64.rpm
```

The packages create the `sys-sentient` service account, install the systemd
unit and drop a config at `/etc/sys-sentient/config.yaml` — marked
`noreplace`, so an upgrade never overwrites your edits. They deliberately do
**not** start the service: the first run mints a one-time setup token that you
need to read from the log to create the first administrator.

```bash
sudo systemctl enable --now sys-sentient
sudo journalctl -u sys-sentient | grep -i "setup token"
```

**Windows / macOS / anything else** — download the archive, extract, run
`sys-daemon`. Verify downloads against `checksums.txt`.

> The shipped unit sets `ProtectKernelLogs=true` and drops all capabilities, so
> `dmesg` collection cannot work under it. journald remains available and is the
> primary log source.

## Platform support

The daemon now **builds for linux, windows and darwin on amd64 and arm64**, as
a static binary with no C toolchain required — the SQLite driver is pure Go.

Metric collection is cross-platform via gopsutil. Log collection and machine
identity now have per-platform implementations:

| | Logs | Machine identity |
|---|---|---|
| Linux | journalctl, dmesg, `/var/log/syslog` | `/etc/machine-id` |
| Windows | System and Application event logs (`wevtutil`) | registry `MachineGuid` |
| macOS | unified log (`log show`) | `IOPlatformUUID` |

Every source fails soft, so a missing or unreadable one costs the others
nothing. Service integration is still Linux-only — the systemd unit ships, and
a Windows service and macOS launchd plist are tracked in
[docs/plans/04-goreleaser-packaging.md](docs/plans/04-goreleaser-packaging.md).

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md). `make verify` runs everything CI runs.

## Licensing

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).

Third-party components and their licences are listed in
[docs/THIRD_PARTY.md](docs/THIRD_PARTY.md). Note that the bundled typefaces
(Sora, Manrope, JetBrains Mono) are under the SIL Open Font License 1.1, and
ship as binaries inside `sys-daemon`, so that notice travels with every copy.

## Documentation

- [CHANGELOG.md](CHANGELOG.md) — what changed
- [SECURITY.md](SECURITY.md) — reporting, and known limitations
- [CONTRIBUTING.md](CONTRIBUTING.md) — development workflow
- [QUICK_START.md](QUICK_START.md) — troubleshooting and operational tips
