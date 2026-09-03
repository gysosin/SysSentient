# Contributing

## Getting set up

```bash
cd web && npm install && npm run build && cd ..
go build -o sys-sentient ./cmd/daemon
./sys-sentient
```

The dashboard is served from `web/dist`, so build the frontend before running
the daemon. For frontend work, `cd web && npm run dev` gives hot reload against
a daemon running separately on :8080.

## Before you open a pull request

Everything CI runs, run locally:

```bash
gofmt -l $(git ls-files '*.go')          # must print nothing
GOTOOLCHAIN=auto go vet ./...
GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
docker build --pull -t sys-sentient:local .
```

## Conventions

- **Tests first.** New behaviour without a failing test first is a defect.
  Table-driven tests, `t.TempDir()` for anything touching the filesystem.
- **Conventional commits**, imperative subject: `feat(alerting): ...`,
  `fix(storage): ...`, `sec(api): ...`, `chore(web): ...`, `docs: ...`.
- **Do not add `Co-Authored-By` lines.**
- Go: `gofmt`, short lowercase package names, exported identifiers documented.
- Frontend: TypeScript function components, two-space indent. Types in
  `web/types.ts`, API calls in `web/services/`, hooks in `web/hooks/`,
  screens in `web/pages/`.

## Things that will bite you

- **Tailwind v4 does not read `tailwind.config.js`** unless `web/index.css`
  declares `@config`. Without it every custom utility silently compiles to
  nothing. `npm run build` runs `verify:css`, which fails the build if the
  design tokens do not reach the bundle. Do not remove that check.
- **Both transports must agree.** Metrics arrive over REST *and* the WebSocket.
  Adding a field means updating `services/api.ts`, `hooks/useWebSocket.ts` and
  `services/normalize.ts` — a field added to only one silently disappears
  depending on connection state.
- **`internal/pii` gates what leaves the machine.** Anything added to the Gemini
  prompt must pass through the scrubber first.
- **Alert evaluation is stateful.** `Evaluator.Evaluate` advances the state
  machine, so it must be called exactly once per sample.

## Do not commit

Agent tooling (`.agents/`, `agent/`, `.claude/`, `CLAUDE.md`,
`skills-lock.json`), build artefacts (`sys-sentient`, `web/dist`), databases
(`*.db*`) and secrets (`.env`, `config.yaml`). All are gitignored.
