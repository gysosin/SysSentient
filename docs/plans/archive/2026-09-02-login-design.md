# Login and sessions — design

**Status:** approved in chat 2026-09-02, implementation in progress
**Replaces:** build-time API key baked into the dashboard bundle
**Depends on:** nothing
**Unblocks:** device enrollment, chart drill-down, users/roles, everything that exposes more surface

## Problem

SysSentient has no login. It has a shared secret that pretends to be one.

- `web/constants.ts:8` reads `VITE_SYS_SENTIENT_API_KEY` at **build** time. Vite
  inlines it into the JavaScript bundle, which the server hands to anyone who
  requests `/`. Whoever can load the page can read the key in the page source.
- `web/constants.ts:19` appends the same key to the WebSocket URL as
  `?api_key=`. It lands in every proxy, load balancer and browser-history log.
- There are no users. There is one key. Revoking it means rebuilding the
  frontend. There is no notion of who did what.
- When the key is unset, `AuthMiddleware.enabled` is false and **every** endpoint
  is open, including `POST /api/analyze`, which spends money.

For a product someone is meant to buy and put on a network, this is
disqualifying. A procurement review stops at "how do users log in".

## Goals

1. Real accounts with hashed passwords and server-issued sessions.
2. The browser never holds a long-lived secret. Nothing auth-related in a URL.
3. Auth is on by default. Turning it off is an explicit, loudly-logged choice.
4. First-run bootstrap that never involves a default password.
5. Two roles from day one — `admin` and `viewer` — enforced server-side.
6. Machine clients (curl, scripts, agents) keep a header-token path.

## Non-goals (this iteration)

- SSO / OIDC / SAML, SCIM, MFA, password reset by email. Catalogued separately.
- Multi-tenancy. Roles are global.
- Per-host authorization.

## Design

### Backend

New package `internal/auth`, with no dependency on `net/http` or storage, so it
can be tested in isolation:

- **Passwords** — argon2id via `golang.org/x/crypto/argon2`, encoded as a PHC
  string (`$argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>`) so parameters travel
  with the hash and can be raised later without a migration. Parameters exceed
  the OWASP minimum (19 MiB, t=2). Verification is constant-time.
  Policy: 12–128 characters, no composition rules (NIST 800-63B).
- **Sessions** — 32 random bytes, base64url. The store keeps only the SHA-256 of
  the token, so a database leak does not yield live sessions. Idle timeout 24 h,
  refreshed on activity; absolute cap 30 days from creation.
- **Roles** — `admin` (everything) and `viewer` (read-only). Mutating endpoints —
  alert rules, acknowledge, analyze, user management — require `admin`.

Storage (`internal/storage`) gains two tables. Timestamps are UTC RFC 3339,
consistent with the rest of the schema:

```sql
CREATE TABLE users (
  id            TEXT PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL CHECK (role IN ('admin', 'viewer')),
  created_at    TEXT NOT NULL,
  last_login_at TEXT
);
CREATE TABLE sessions (
  token_hash   TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at   TEXT NOT NULL,
  expires_at   TEXT NOT NULL,
  last_seen_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);
```

Endpoints (`internal/server/auth_handlers.go`):

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET  | `/api/auth/setup`    | public | `{ "needsSetup": true }` while zero users exist |
| POST | `/api/auth/setup`    | public + one-time token | create the first admin |
| POST | `/api/auth/login`    | public, rate-limited | `{email,password}` → session cookie |
| POST | `/api/auth/logout`   | session | revoke this session, clear cookie |
| GET  | `/api/auth/me`       | session | current user `{id,email,role}` |
| POST | `/api/auth/password` | session | change own password; revokes other sessions |
| GET  | `/api/users`         | admin | list |
| POST | `/api/users`         | admin | create `{email,password,role}` |
| DELETE | `/api/users/{id}`  | admin | delete; refuses self and the last admin |

Behaviour that matters:

- **Bootstrap.** On start, if the user count is zero, the daemon generates a
  32-byte setup token, holds it in memory only, and logs a single line:
  `first-run setup: open http://<addr>/setup and enter token <token>`. The
  token is consumed on success and never persisted. There is no default
  password, ever.
- **Cookie.** Name `sys_session`; `HttpOnly`, `SameSite=Strict`, `Path=/`.
  `Secure` is set when the request arrived over TLS or `X-Forwarded-Proto:
  https` — set unconditionally it would silently break every plain-HTTP LAN
  deployment, since browsers drop `Secure` cookies on `http://`. The daemon logs
  a warning at startup when TLS is off.
- **Login hardening.** Generic `401 invalid credentials` whether the email
  exists or not. When it does not, a dummy hash is still verified so the
  response time does not enumerate users. Per-IP token bucket, 5 attempts/min.
- **CSRF.** `SameSite=Strict` blocks cross-site cookie sends in every current
  browser. Belt and braces: mutating requests with a `Sec-Fetch-Site` header of
  `cross-site` are rejected before any handler runs.
- **Middleware.** `/api/*` and `/ws/*` accept **either** a valid session cookie
  **or** a valid `X-API-Key` / `Bearer` header (the existing machine token,
  kept for scripts and the agent). The `?api_key=` query parameter is removed
  from the WebSocket path entirely. `/health`, `/metrics`, static assets, and
  the auth endpoints above stay public.
- **Always on.** The "no key configured → everything open" mode is gone. The
  only way to run without auth is `server.insecure: true`
  (`SYS_SENTIENT_SERVER_INSECURE=1`), which logs a warning on every start.
- **Session sweep.** Expired sessions are deleted by the existing retention
  loop.

### Frontend

- `hooks/useAuth.tsx` — an `AuthProvider` with a four-state machine:
  `loading → setup | anon | authed`. On mount it calls `/api/auth/me`; on 401 it
  checks `/api/auth/setup` to decide between `/setup` and `/login`. Any later
  401 from the API drops the app back to `anon`.
- Routes: `/login` and `/setup` are public. Everything else is wrapped in
  `<RequireAuth>`, which redirects and preserves the intended destination.
- `pages/Login.tsx`, `pages/Setup.tsx` — shadcn `Card` + form. Labels tied to
  inputs, `autocomplete="email" / "current-password" / "new-password"`,
  `aria-invalid` + `aria-describedby` on error, submit disabled while pending,
  error announced via `role="alert"`.
- `AppShell` gains a user menu (Radix dropdown): email, role badge, change
  password, sign out.
- Settings gains a **Users** tab for admins: list, create, delete, with the
  last-admin guard surfaced as a disabled control rather than a server error.
- `constants.ts` loses `API_KEY` and the `VITE_SYS_SENTIENT_API_KEY` env var.
  `fetch` uses `credentials: 'same-origin'`. The WebSocket URL has no query.

### Configuration

```yaml
server:
  insecure: false          # true disables auth entirely; warned on every start
auth:
  session_idle_hours: 24
  session_max_days: 30
  login_rate_per_minute: 5
```

`server.api_key` keeps its name and meaning as the machine token.
`server.agent_key` is unchanged.

## Migration

An existing deployment that upgrades sees `/setup` on first load, creates an
admin, and continues. Scripts using `X-API-Key` keep working. The only thing
that stops working is the removed `?api_key=` WebSocket query — no supported
client used it except the old bundle, which is replaced by the same deploy.

## Testing

`internal/auth`: hash/verify round-trip; wrong password; tampered and malformed
PHC strings; parameters survive encode/decode; session token uniqueness;
expiry, idle refresh, absolute cap; revoke.

`internal/server`: login sets a cookie with `HttpOnly` and `SameSite=Strict`;
`Secure` present only under TLS / forwarded https; wrong password and unknown
email both return the same 401 body; `/me` 200 with cookie, 401 without; logout
invalidates; setup succeeds once with the token and 409s once a user exists;
login is rate-limited; viewer gets 403 on `POST /api/alerts/rules`; `X-API-Key`
still authenticates; WebSocket upgrade authenticates from the cookie and
rejects `?api_key=`; `Sec-Fetch-Site: cross-site` POST is rejected; deleting
the last admin is refused.

`web`: `AuthProvider` state transitions from mocked responses; `RequireAuth`
redirects; login form submits, shows error, disables while pending, focus lands
on the error.

## Risks

| Risk | Mitigation |
|---|---|
| Existing no-key deployments lose open access | Setup flow on first load; `server.insecure` escape hatch |
| `Secure` cookie breaks plain-HTTP LAN installs | Per-request detection; startup warning |
| argon2 memory on tiny hosts (64 MiB per attempt) | Login rate limit bounds concurrent hashes |
| Session table growth | Swept by the retention loop |

## Files

New: `internal/auth/{password,session,user}.go` (+tests),
`internal/server/auth_handlers.go` (+tests), `web/hooks/useAuth.tsx`,
`web/pages/{Login,Setup}.tsx`, `web/components/{RequireAuth,UserMenu}.tsx`,
`web/components/ui/{label,dropdown-menu,dialog}.tsx`.

Modified: `internal/server/{auth,server,websocket}.go`,
`internal/storage/sqlite.go`, `internal/config/config.go`,
`cmd/daemon/main.go`, `web/{App,constants}.tsx|ts`, `web/services/api.ts`,
`web/hooks/useWebSocket.ts`, `web/components/AppShell.tsx`,
`web/pages/Settings.tsx`, `config.yaml.example`, `README.md`, `SECURITY.md`,
`CHANGELOG.md`.

New dependency: `golang.org/x/crypto` (BSD-3, Go team). No argon2 in the
standard library; bcrypt would also live here.
