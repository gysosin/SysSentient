# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Server mode was destroying the history it promised to keep.** The fleet
  deployment ran a maintenance list that pruned raw metrics without ever rolling
  them up first, so everything past the retention cutoff was deleted rather than
  aggregated: the 30-day and 1-year tiers never existed, `/api/export` returned
  nothing, and — with no WAL checkpoint or vacuum — the database only grew. Both
  modes now run one shared maintenance path, guarded by a test that drives the
  real server loop.
- Alerts were evaluated, stored and dispatched on the ingest path even with
  `alerting.enabled: false`. The evaluator is now built only when alerting is
  on, so every caller inherits the setting.
- `GET`/`PATCH /api/settings` returned 404 in server mode, because the runtime
  was only created on the all-in-one path — so nothing was tunable on the
  deployment most likely to be tuned remotely.
- Unredeemed join tokens accumulated for the life of an install;
  `PruneExpiredJoinTokens` was never called outside its own test.
- `alert_events` now records `host_id` as well as `hostname`, so alert history
  can be joined to hosts and two machines sharing a name stop rendering as
  duplicate alerts.

### Added

- **`sys-sentient service install|uninstall|status`**, and `--install-service`
  on `agent join`, so an enrolled machine survives logout and reboot. systemd
  on Linux (system or `--user` scope), launchd on macOS, the service manager on
  Windows — which also required the daemon to answer service control requests,
  without which the SCM starts it and then marks it failed. `service status`
  distinguishes a slow start from a crash loop and names the command to read
  the logs.

- **Enrol a machine from the dashboard.** Settings → Devices issues a
  single-use join token and shows the exact command to run on the new machine;
  `sys-sentient agent join --server <url> --token <t>` exchanges it for a
  credential belonging to that machine alone, writes `agent.yaml` with mode
  `0600`, and prints how to start it. The device appears on the Devices screen
  once it reports, with its version and last-seen.
- **Per-agent credentials and revocation.** Previously the whole fleet shared
  one static key: there was no per-machine identity, and withdrawing one
  machine's access meant re-keying every machine at once. Revoke now takes
  effect immediately for one device. Only credential hashes are stored, never
  the credentials themselves.
- `server.public_url`, so the enrolment command is correct behind a reverse
  proxy, where the request's `Host` is the proxy's own name.
- Wiki pages — Home, FAQ, Adding a machine, Troubleshooting — written and
  version-controlled in `docs/wiki/`, so they are reviewed with the code.

### Changed

- **The installed binary is now `sys-sentient`, not `sys-daemon`.** Everything
  else already used the product name — the systemd unit, the package, the
  config directory, the docs — so the old name meant the enrolment command the
  dashboard tells you to paste (`sys-sentient agent join …`) named a binary
  that did not exist on an installed system. The packages ship a `sys-daemon`
  symlink so existing scripts and units keep working.

- **The agent spool is append-only.** Every sample used to be written by
  reading, decoding, re-encoding and rewriting the whole file: **230 ms per
  sample** against a full 5,000-sample buffer, or 11% of a 2-second poll
  interval, sustained for the entire outage the buffer exists to survive. It is
  now **0.48 ms** amortised, including compaction — **484× faster**. Spools
  written by earlier versions are read and preserved on upgrade.
- `/api/ingest` no longer rejects unknown JSON fields. A newer agent sending a
  field an older server has never heard of had its entire batch rejected with a
  400, which turned any staggered fleet upgrade into an outage.
- A revoked or unrecognised agent now logs one actionable error naming the fix,
  instead of a generic warning that looks like a transient network problem.
- **Metric rows are 15.1% smaller** (2249 → 1909 bytes, measured per column
  over a real database). `top_processes` was `processes` rendered as text — the
  identical data stored twice on every sample — and is now derived on read, so
  no API shape changed and no history was dropped. Filesystem entries drop
  `free_bytes` and `used_percent` only where they genuinely match the
  derivation from `total_bytes` and `used_bytes`, which on ext4 and btrfs they
  do not: reserved blocks mean free space is not total minus used.

### Fixed

- **AI analysis failed outright against a live Gemini key.** The prompt asked
  for bullet points, so the model returned `detailedAnalysis` as an array —
  a reasonable reading — and the whole analysis was discarded with
  `cannot unmarshal array into Go struct field`. Free-text fields now accept a
  string, an array or nested objects and render them as text, and the request
  sends an explicit response schema so the shape is pinned rather than implied.

- **The packaged systemd service could not start.** The unit's
  `ExecStart=/opt/sys-sentient/sys-daemon` named a path no package ever
  created — the binary installs to `/usr/bin`. Anyone who installed the `.deb`,
  `.rpm` or `.apk` from v0.1.0-beta.1 and ran `systemctl start sys-sentient`
  got a failure. Verified by building the packages and checking that the unit's
  `ExecStart` resolves to a file the package actually contains.

- **An upgraded agent could silently stop reporting.** A spool written in the
  old format and then appended to in the new one parsed as neither, so the
  agent buffered indefinitely, sent nothing, and logged nothing at all. Both
  formats are now read, including a file that contains each in turn.
- A damaged spool line no longer discards the whole buffer, and a spool
  truncated mid-write by a crash or a full disk no longer corrupts the next
  sample appended after it.
- The Devices screen showed "expires 0s from now" for every new token: the
  countdown measured elapsed time against a future timestamp.

## [0.1.0-beta.1] - 2026-09-03

First tagged release. The repository had 100+ commits and zero tags, so
everything below is the initial published state rather than a delta.

### Added

- **Documentation, a website, and the repository metadata a project needs to be
  usable by anyone else.** `docs/` now covers install, configuration,
  architecture, deployment, performance, privacy, development and releasing,
  each topic with exactly one home — configuration had previously been
  documented in three places and build commands in five. A static site deploys
  to GitHub Pages behind a link-and-metadata check, so a broken landing page
  fails the build rather than going unnoticed. Plus a code of conduct, issue
  and pull-request templates, and a rewritten README.

- **Poll interval, retention and log level are editable from Settings and take
  effect immediately.** Previously every setting was read once at boot, so
  changing how often the machine is sampled meant editing a file and restarting
  the daemon — which also discards the state you were investigating. Verified
  live: 3 samples in 6s at a 2s interval, then 8 samples in 8s after the
  change, with no restart.

- **Backup and export.** `sys-daemon --backup <path>` writes a consistent copy
  while the daemon is running — copying the file with `cp` produces a corrupt
  result, because the database and its write-ahead log must agree — restricts
  it to `0600` since it holds password hashes and session tokens, and verifies
  it with `PRAGMA integrity_check` before reporting success. `GET /api/export`
  serves any retention tier as CSV or JSON, so a year of history is portable
  rather than trapped in one file on one host.

- **Tiered retention, so history is kept rather than deleted.** Metrics were
  hard-`DELETE`d after 24 hours, so "was this machine slow last Tuesday" had no
  answer. Full-resolution samples are now rolled up into per-minute averages
  kept for 30 days and per-five-minute averages kept for a year; data only
  leaves the system at the end of the last tier. Measured: an hour of raw
  samples is 3,808 KB and 88 KB once rolled up — 2.3%. Both tiers are
  configurable.

- **Every Overview tile opens a drill-down.** A tile says the machine is at 90%;
  the drill-down says of what and where — per-core distribution and the ranked
  processes responsible for CPU, memory holders and swap pressure for memory,
  per-filesystem capacity with inode usage for disk, and history for each. The
  tiles are buttons, so they are reachable from the keyboard, and Escape closes
  the view.

- **Cross-platform packages.** A release now produces static binaries for
  linux, windows and darwin on amd64 and arm64, plus `.deb`, `.rpm` and `.apk`
  with a service account, systemd unit and a `noreplace` config, and a
  multi-arch container image. Previously a release produced exactly one
  artifact — a linux/amd64 tarball — because the cgo SQLite driver made
  anything else impossible.
- CI now builds and vets on Windows and macOS runners. Those targets had never
  been compiled, so a break in their platform-specific code could only have
  been found by a user on that platform.

- **Apache-2.0 licence**, with `NOTICE` and `docs/THIRD_PARTY.md`. The project
  was previously "all rights reserved" by default, which meant nobody could
  legally redistribute or run it — a hard blocker on shipping packages at all.
  The bundled typefaces are SIL OFL-1.1 and ship as binaries inside the daemon,
  so that notice is carried explicitly.

- **Windows and macOS support for log collection and machine identity.** Log
  sources are now a per-platform list: journalctl/dmesg/syslog on Linux, the
  System and Application event logs via `wevtutil` on Windows, and the unified
  log via `log show` on macOS. Machine identity likewise reads
  `/etc/machine-id`, the registry `MachineGuid`, or `IOPlatformUUID`. Without
  the last of these, a rename on Windows silently started a new history.

- **Login and user accounts** (`internal/auth`, `internal/server/auth_handlers.go`,
  `internal/server/user_handlers.go`) — argon2id password hashing encoded as PHC
  strings, server-issued session cookies (`HttpOnly`, `SameSite=Strict`,
  `Secure` under TLS), `admin`/`viewer` roles enforced server-side, a one-time
  first-run setup token so no default password ever exists, and admin user
  management. New endpoints: `GET|POST /api/auth/setup`, `POST /api/auth/login`,
  `POST /api/auth/logout`, `GET /api/auth/me`, `POST /api/auth/password`,
  `GET|POST /api/users`, `DELETE /api/users/{id}`.
- **Login, first-run setup and account screens** in the dashboard, plus a
  header account menu and a Users tab in Settings.

- **Alerting engine** (`internal/alerting`) — threshold rules with a required
  duration, a pending → firing → resolved state machine, acknowledgement, and
  webhook plus Slack notification channels. Six default rules cover CPU, memory,
  swap, disk, load and temperature. Alert transitions are persisted and exposed
  at `GET /api/alerts`, `/api/alerts/rules` and `/api/alerts/history`.
- **Filesystem capacity metrics** — per-mount usage, free space and inode usage.
  Previously `disk.Usage()` was never called, so the product could not detect a
  full disk at all.
- **Navigation and six screens** — Overview, Processes, Logs, AI Insights,
  Alerts and Settings, with client-side routing and an SPA fallback on the
  server. The dashboard was previously a single page with no navigation.
- **Prometheus scrape endpoint** at `GET /metrics`, including the daemon's own
  goroutine count, heap and GC stats.
- **Structured logging** (`log/slog`) with configurable level and text/JSON
  format.
- **Version identity** — `--version` flag, and version plus commit reported by
  `/health`.
- **Rate limiting** on `POST /api/analyze` and `GET /api/logs`.
- **Security headers** — CSP, `X-Frame-Options`, `X-Content-Type-Options`,
  `Referrer-Policy`, `Cross-Origin-Opener-Policy`.
- Host identity (`hostname`) and real host uptime on every sample.
- `--config` flag for an explicit config path.
- `config.yaml.example`, `CONTRIBUTING.md`, `SECURITY.md`, `.golangci.yml`,
  Dependabot configuration.

### Changed

- **SQLite concurrency and disk reclamation.** The connection pool was capped
  at one, which enabled WAL and then forfeited the concurrent reads that are
  its only benefit — every dashboard read serialised behind every write. Agent
  ingest wrote one autocommit transaction per sample, so a 60-sample flush was
  60 fsyncs contending for a single write lock; batched, it is 2.1× faster. The
  WAL is now checkpointed on every maintenance tick (it had been measured
  larger than the database itself) and the file is vacuumed daily, so deleted
  pages return to the filesystem instead of only to SQLite's freelist.

- **Idle CPU cut from 4.10% of a core to 0.78%**, and garbage per collection
  from 14.8 MB to 0.78 MB. Two causes: a 500 ms sleep held the main loop for a
  quarter of every two-second tick, and gopsutil's `Times()` was called once
  per process — 594 times per poll — allocating ~50 objects each to read two
  integers that `/proc/<pid>/stat` gives directly. See `docs/PERFORMANCE.md`
  for the numbers and how to reproduce them.

- **Settings is organised into sections** — Status, Configuration, Privacy &
  integrations, Users, Account — instead of six panels stacked in one grid,
  which put the change-password form beside the API endpoint reference. Each
  section is addressable by URL, so the existing `/settings#account` link from
  the account menu keeps working, and viewers no longer see a Users tab at all
  rather than seeing one that does nothing.

- **Readable type and a full-width layout.** 72 hardcoded font sizes between
  9px and 13px are replaced by a named scale with a 12px floor; log timestamps
  in particular were unreadable. The shell no longer caps at 1600px, which left
  roughly a third of a 2560px display unused on a product whose job is dense
  information. A test now fails the build on any font size below the floor or
  any fixed pixel cap in the shell — this had already regressed twice.

- **The dashboard is embedded in the binary** (`web/embed.go`). The server
  previously read it from `./web/dist` relative to the working directory, which
  is why the systemd unit had to pin `WorkingDirectory` — and why no `.deb`,
  `.rpm` or `.msi` could ship it. `sys-daemon` now serves the UI from any
  directory. A disk copy at `./web/dist`, when present, still wins so that
  `npm run build` is picked up without recompiling the daemon.

- Frontend toolchain raised: **Vite 6 → 8** (rolldown), **Vitest 3 → 5**,
  **recharts 2 → 3**, React 19.2.3 → 19.2.8. Build time dropped from ~8s to
  ~0.8s. Two breaking changes needed fixing: rolldown's `manualChunks` only
  accepts a function, and recharts 3 widened its tooltip formatter signatures.

- **The SQLite driver is now pure Go** (`modernc.org/sqlite` replacing
  `mattn/go-sqlite3`). `CGO_ENABLED=0` builds succeed for linux, windows and
  darwin on both amd64 and arm64 — previously only linux/amd64 built at all,
  and only with a C toolchain. Measured cost: writes 120µs → 165µs per sample,
  which is 0.008% of a two-second poll interval.

- `/api/*` and `/ws/*` now require a session cookie or the `X-API-Key` machine
  token. The previous "no key configured means everything is open" mode is
  gone; `server.insecure: true` is the explicit, loudly-warned escape hatch.
- The default `server.allowed_origins` now includes `http://localhost:3000`,
  the port this repo's Vite config actually uses. It previously listed only
  Vite's default 5173, so every `npm run dev` session failed the WebSocket
  upgrade with a 403.

- Dashboard redesigned from the neon/CRT theme to a professional, information-
  first system with WCAG-compliant contrast, visible focus states, `aria-live`
  regions and `prefers-reduced-motion` support.
- Top process count is configurable (`collector.top_processes`, default 10) —
  previously hardcoded to 3.
- Frontend test harness migrated from `node --test` to Vitest with Testing
  Library and jsdom.

### Removed

- `VITE_SYS_SENTIENT_API_KEY`. Vite inlined it into the published bundle, so
  the key was readable by anyone who could load the dashboard.
- The `?api_key=` WebSocket query parameter, which leaked the key into proxy
  and browser-history logs.

### Fixed

- **The hosts table was empty on every single-node install.** `UpsertHost` was
  called only from the agent ingest path, so an all-in-one daemon — the default
  — accumulated metrics while `GET /api/hosts` returned `[]`; measured against a
  live database at 41,553 metrics rows and zero hosts. The dashboard hid it
  behind a `hosts.length || 1` fallback, which is now removed, so a zero there
  means something is genuinely wrong.

- **Timestamps were stored in a format SQLite could not read, and the migration
  that tried to normalise them destroyed data.** `database/sql` leaves
  parameter encoding to the driver, and `modernc.org/sqlite` writes a
  `time.Time` as Go's `String()` form — `2026-09-03 17:20:12.19135217 +0000
  UTC` — which `datetime()` cannot parse. The normalisation migration then
  wrote that `NULL` back: on the nullable `metrics.timestamp` it silently
  destroyed 158 rows, and on the `NOT NULL` `alert_events.occurred_at` it
  aborted start-up entirely, so the daemon refused to boot. Timestamps are now
  written through one explicit layout, and the migration leaves anything it
  cannot parse untouched instead of nulling it.

- A `<div>` nested inside a `<p>` in the Overview stat cards, which React
  reported as a hydration-breaking HTML nesting error whenever a card was in
  its loading state.

- **The process table was permanently empty.** Process data was only populated
  by the REST polling path, which is skipped whenever the WebSocket is
  connected — so the table filled in only when the socket was *down*.
- **The design system never compiled.** Tailwind v4 does not auto-load
  `tailwind.config.js`; without an `@config` directive all 49 custom utility
  usages silently became no-ops. The AI panel's recommended actions were stuck
  at `opacity: 0` and rendered invisible, and alert styling was inverted.
- **Per-process CPU was a lifetime average**, so a process that spiked an hour
  ago outranked one busy right now. Now derived from the delta between polls.
- **`gemini.max_daily_cost` did nothing.** The setting was defined, defaulted
  and validated, then read by no code, while the docs described it as a spend
  cap. Now enforced per UTC day.
- **The PII scrubber had no IPv6 support** and was never applied to process
  names, which are interpolated directly into the Gemini prompt.
- **`/api/logs` ignored the privacy configuration**, hardcoding all-masking on
  one code path while the daemon honoured config on another.
- Uptime shown in the dashboard was a client-side counter that reset on page
  refresh, not host uptime.
- The dashboard could not distinguish healthy from stale from dead: failures
  were swallowed and last-known-good values were shown indefinitely.
- Charts had no time axis and blanked the tooltip label, so a spike could not be
  located in time.
- `Hub.Run()` had no exit path and leaked its goroutine on every shutdown,
  leaving clients connected to a dead server.
- `/health` reported "healthy" with a wedged collector; it now reports collector
  liveness and sample age.
- `GET /api/metrics` and `/api/insights` returned JSON `null` instead of `[]`
  for empty result sets.
- `insights` had no index despite being queried with `ORDER BY timestamp DESC`
  on every dashboard poll.
- Frontend tests at the `web/` root were silently skipped: the harness used `**`
  under `sh`, which is not recursive. Replaced with Vitest.
- `npm audit` and tests ran inside the Dockerfile, making image builds
  non-hermetic and breaking previously-green commits when new advisories landed.

### Security

- **Password verification now decodes the stored hash strictly.** Go's
  non-strict base64 decoder ignores the trailing padding bits of the final
  character, and a 32-byte argon2id key encodes to 43 characters whose last one
  carries only 4 significant bits. Two different stored strings therefore
  decoded to the same key and both verified. Now rejected as a malformed hash.
- Go toolchain raised to 1.25.13. `govulncheck` reported **9 standard-library
  vulnerabilities reachable from this code** on 1.25.10, including
  `crypto/x509` certificate verification (reached from `Server.Start`) and
  `net/http` Punycode handling (reached from the agent's push path). Now zero.
- Authentication is now required on every `/api/*` and `/ws/*` route. The build
  time dashboard key that Vite inlined into the published bundle, and the
  `?api_key=` WebSocket query parameter, are both gone — see **Removed**.
- See [SECURITY.md](SECURITY.md) for the current threat model and the
  limitations that remain, notably that the daemon serves plain HTTP and must
  sit behind a TLS terminator.

### Known limitations

- Single-host only. There is no host dimension in the ingest path.
- No TLS; run behind a reverse proxy.
- Alert rules are built in and not yet editable from the browser.

[Unreleased]: https://github.com/gysosin/SysSentient/compare/v0.1.0-beta.1...HEAD
[0.1.0-beta.1]: https://github.com/gysosin/SysSentient/releases/tag/v0.1.0-beta.1
