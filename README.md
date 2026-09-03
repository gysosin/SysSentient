# SysSentient

**Self-hosted server monitoring that stays honest.**

Live metrics every two seconds, alerting that ignores transient spikes, and an
AI that explains what went wrong — with the evidence it used and the data it
redacted first.

[![CI](https://github.com/gysosin/SysSentient/actions/workflows/ci.yml/badge.svg)](https://github.com/gysosin/SysSentient/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gysosin/SysSentient?include_prereleases)](https://github.com/gysosin/SysSentient/releases)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Go Report](https://goreportcard.com/badge/github.com/gysosin/SysSentient)](https://goreportcard.com/report/github.com/gysosin/SysSentient)

[Website](https://gysosin.github.io/SysSentient/) ·
[Documentation](docs/README.md) ·
[Install](docs/INSTALL.md) ·
[Changelog](CHANGELOG.md)

> **Status:** pre-1.0. The daemon serves plain HTTP — terminate TLS in front of
> it and read [SECURITY.md](SECURITY.md) before letting anything but localhost
> reach it.

## Why

Most monitoring either costs more than it observes, or tells you a number
without telling you what caused it. SysSentient is one static binary that idles
at **0.78% of a core**, keeps a year of history, and when something breaks will
name the process responsible.

- **Truthful about its own data.** The feed indicator distinguishes live,
  polling, stale and no-data. The worst thing a monitor can do is show
  confident numbers that stopped being true ten minutes ago.
- **Alerts with a duration.** A rule must hold before it fires, so a
  `make -j16` does not page anyone.
- **AI that shows its work.** Suggestions are labelled as the model's claim,
  never executed automatically, and the payload is redacted before it leaves.
- **Drill down to the cause.** A tile says 90%; clicking it shows per-core
  distribution, the ranked processes responsible, and inode pressure beside
  capacity.
- **Yours entirely.** No telemetry, no phone-home, no CDN. It runs air-gapped.

## Install

```bash
sudo dpkg -i sys-sentient_<version>_linux_amd64.deb   # Debian / Ubuntu
sudo rpm  -i sys-sentient_<version>_linux_amd64.rpm   # Fedora / RHEL

sudo systemctl enable --now sys-sentient
sudo journalctl -u sys-sentient | grep -i "setup token"
```

Open `http://localhost:8080` and use the token to create the first
administrator. There is no default password at any point.

Builds exist for **linux, windows and darwin on amd64 and arm64**, plus a
multi-arch container image. Full instructions, including from source, are in
[docs/INSTALL.md](docs/INSTALL.md).

## What it costs

Measured on an 8-core workstation at the default two-second interval.
Reproduce it with the procedure in [docs/PERFORMANCE.md](docs/PERFORMANCE.md).

| | |
|---|---|
| Idle CPU | **0.78%** of one core |
| Resident memory | 26 MB |
| Collection | 20 ms per sample, 594 processes |
| Binary | 31 MB, static, dashboard included |

## Screens

Overview, Processes, Logs, AI Insights, Alerts and Settings, plus sign-in and
first-run setup. Dark by default, because this is left open on a second monitor.

## API

| Endpoint | Auth | Description |
|---|---|---|
| `GET /health` | public | Status, version, database and collector liveness |
| `GET /metrics` | public | Prometheus exposition |
| `GET /api/metrics` | key | Recent samples |
| `GET /api/export` | key | Retained history as CSV or JSON |
| `GET /api/insights` | key | Recent AI insights |
| `POST /api/analyze` | admin | Trigger AI analysis (rate limited — costs money) |
| `GET /api/logs` | key | Recent system logs (rate limited) |
| `GET /api/alerts` | key | Pending and firing alerts |
| `GET /api/alerts/rules` | key | Configured rules |
| `GET /api/alerts/history` | key | Recent transitions |
| `POST /api/alerts/{ruleID}/acknowledge` | admin | Silence an active alert |
| `GET /api/hosts` | key | Known hosts |
| `GET /api/settings` | key | Settings that apply without a restart |
| `PATCH /api/settings` | admin | Change them live |
| `GET /ws/metrics` | key | Live metric stream |

## Configuration

Copy [`config.yaml.example`](config.yaml.example), or use `SYS_SENTIENT_*`
environment variables. Poll interval, retention and log level are editable from
the dashboard and apply immediately; everything else is in
[docs/CONFIGURATION.md](docs/CONFIGURATION.md).

## Fleets

One server, many agents. Agents collect locally and push over authenticated
HTTPS, buffering to disk through a network outage.

Adding a machine is two steps. In **Settings → Devices**, name it and press
*Generate command*; then run what it gives you on that machine:

```bash
sys-sentient agent join --server https://monitor.example.com --token <token> --install-service
```

That registers a service too, so the machine keeps reporting after a reboot.
The token is single-use and expires in an hour. It buys a credential belonging
to that machine alone, which you can revoke on its own without touching the
rest of the fleet. See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Backup

Do not copy the database file while the daemon is running — SQLite keeps the
database and its write-ahead log as two files that must agree, and a copy taken
between them is corrupt.

```bash
sys-sentient --backup /var/backups/sys-sentient-$(date +%F).db
```

## Privacy

Nothing leaves the machine unless `gemini.api_key` is set. With AI enabled, IP
and e-mail addresses and home-directory usernames are redacted before anything
is sent. See [docs/PRIVACY.md](docs/PRIVACY.md).

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) and [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).
`make verify` runs everything CI runs.

## Licence

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE). Third-party
components are listed in [docs/THIRD_PARTY.md](docs/THIRD_PARTY.md); note the
bundled typefaces are SIL OFL-1.1 and ship inside the binary.
