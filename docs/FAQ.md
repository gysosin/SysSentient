# FAQ

## Does it need a database server?

No. SQLite, in a single file at `database.path`. Nothing to install.

## How much history does it keep?

A year, in three tiers: raw samples for 24 hours, per-minute averages for 30
days, per-five-minute averages for a year. All three are configurable. Older
tiers are weighted averages, not samples thrown away — a minute's average is
weighted by how many samples it summarises, so a gap does not distort it.

## Does it send my data anywhere?

Only if you enable the AI analysis, and then only what that analysis needs.
Usernames, emails, IPs and paths are scrubbed before anything is sent. With
`ai.enabled: false` nothing leaves the machine at all. See
[PRIVACY.md](PRIVACY.md).

## What does it cost to run?

0.78% of one core at idle on a 2-second poll interval, and about 1.9 KB per
sample on disk before rollups. Both are measured, with the method written down,
in [PERFORMANCE.md](PERFORMANCE.md).

## Can I change the poll interval without restarting?

Yes. Poll interval, retention and log level are editable in **Settings →
Configuration** and apply immediately.

## Does it work on Windows and macOS?

Yes. Static binaries for linux, windows and darwin on amd64 and arm64, plus
`.deb`, `.rpm` and `.apk` packages and a multi-arch container image. There is
no C toolchain requirement — the SQLite driver is pure Go.

## Is there an API?

Yes, and the dashboard uses nothing else. `GET /api/metrics`, `/api/hosts`,
`/api/alerts`, `/api/export` (CSV or JSON), plus `/metrics` in Prometheus
exposition format. Authenticate with a session cookie or an `X-API-Key` header.

## How do I back it up?

```bash
sys-sentient --backup /var/backups/sys-sentient-$(date +%F).db
```

Do not copy the database file while the daemon is running. SQLite keeps the
database and its write-ahead log as two files that must agree, and a copy taken
between them is corrupt. The `--backup` flag uses SQLite's online backup, which
is safe on a live database.
