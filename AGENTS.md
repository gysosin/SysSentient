# Repository Guidelines

SysSentient is a Go daemon with a React dashboard, distributed as one static
binary with the UI embedded.

## Structure

- `cmd/daemon/` — entry point, the collect loop, graceful shutdown
- `internal/collector/` — metrics via gopsutil, plus a direct `/proc` reader on Linux
- `internal/storage/` — SQLite, migrations, tiered rollups, backup
- `internal/server/` — HTTP routes, auth, WebSocket, ingest, export
- `internal/alerting/` — rules, the pending→firing→resolved machine, notification
- `internal/auth/` — argon2id, sessions, roles, the first-run token
- `internal/ai/`, `internal/pii/` — Gemini analysis and the redaction that gates it
- `internal/agent/` — push client and its on-disk spool
- `internal/logs/`, `internal/hostid/` — per-platform, behind build tags
- `internal/config/`, `internal/logging/`, `internal/version/`
- `web/` — React 19 dashboard, embedded via `web/embed.go`
- `docs/` — documentation; `docs/plans/` — the work, sharded

Tests live beside the code as `*_test.go`. Built artefacts and local data
(`sys-sentient`, `web/dist/`, `*.db`, `.env`, `config.yaml`) stay untracked.

## Commands

`make help` lists every target. The ones that matter:

```bash
make verify     # everything CI runs
make build      # dashboard, then daemon — in that order, the UI is embedded
make lint       # golangci-lint, pinned to the CI version
make vuln       # govulncheck
```

Frontend: `cd web && npm run dev` (port 3000, proxies the API). A real
`web/dist` on disk wins over the embedded copy, so changes appear without
recompiling the daemon.

## Style

`gofmt` for Go; short lowercase package names; PascalCase exported,
camelCase unexported. TypeScript/React function components, ESM imports, two
spaces. UI types in `web/types.ts`, API calls in `web/services/`, hooks in
`web/hooks/`.

Comments explain *why*, not *what*. A comment restating the code is noise; one
recording a decision or a trap is worth its space.

## Testing

Add tests beside the code you changed. Confirm a new test fails without your
change — a test that cannot fail is not a test.

Backend: `go test ./... -race`. Frontend: Vitest with Testing Library and jsdom.
Both run in `make verify`.

## Commits and pull requests

Conventional commits, imperative subjects: `feat(...)`, `fix(...)`, `sec(...)`,
`perf(...)`, `chore(...)`, `docs(...)`. **No `Co-Authored-By` lines.**

Update `README.md`, `CHANGELOG.md` and the relevant `docs/` page in the same
commit as the change they describe. One shard of work per branch.

## Security

Never commit API keys, `.env`, `config.yaml`, databases, binaries or `web/dist`.
Configure secrets through `SYS_SENTIENT_*` environment variables. Anything added
to the Gemini prompt must pass through `internal/pii` — that is the egress gate.
