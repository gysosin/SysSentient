# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
- **No LICENSE file yet** — the project is "all rights reserved" by default
  until one is chosen.

[Unreleased]: https://github.com/gysosin/SysSentient/commits/main
