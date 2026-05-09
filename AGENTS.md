# Repository Guidelines

## Project Structure & Module Organization

SysSentient is a single-node system monitor with a Go daemon and a React/Vite dashboard.

- `cmd/daemon/`: daemon entrypoint; starts collection, storage, API, WebSocket, and AI analysis.
- `internal/collector/`: system metrics collection via `gopsutil`.
- `internal/server/`: HTTP routes, API-key auth, CORS, and WebSocket streaming.
- `internal/storage/`: SQLite persistence and schema migration helpers.
- `internal/ai/`, `internal/pii/`, `internal/logs/`: Gemini analysis, privacy scrubbing, and system log collection.
- `web/`: React UI, Vite config, Tailwind styling, API services, hooks, and components.
- Tests live beside Go packages as `*_test.go`. Built artifacts and local data such as `sys-daemon`, `web/dist/`, `*.db`, and `.env` must stay untracked.

## Build, Test, and Development Commands

- `cd web && npm install`: install frontend dependencies.
- `cd web && npm run dev`: run the Vite dev server on port `3000`.
- `cd web && npm audit --audit-level=moderate`: check frontend dependency advisories.
- `cd web && npm test`: run frontend unit tests with Node's test runner.
- `cd web && npm run typecheck`: run strict TypeScript checks.
- `cd web && npm run build`: build dashboard assets into `web/dist/`.
- `GOTOOLCHAIN=auto go build -o sys-daemon ./cmd/daemon`: build the daemon binary with the Go version from `go.mod`.
- `./sys-daemon`: run the API server and dashboard at `http://localhost:8080`.
- `GOTOOLCHAIN=auto go vet ./...`: run backend static analysis.
- `GOTOOLCHAIN=auto go test ./... -v`: run all backend tests.
- `GOTOOLCHAIN=auto go run golang.org/x/vuln/cmd/govulncheck@latest ./...`: scan called Go code for known vulnerabilities.

For a production-style local run, build `web/dist` before starting `sys-daemon`.

## Coding Style & Naming Conventions

Use `gofmt` for Go changes: `gofmt -w <files>` or `go fmt ./...`. Keep package names short and lowercase. Exported identifiers use PascalCase; unexported identifiers use camelCase.

Frontend code uses TypeScript/React function components, ESM imports, and two-space indentation. Keep UI types in `web/types.ts`, API calls in `web/services/`, and hooks in `web/hooks/`.

## Testing Guidelines

Add Go tests next to changed packages using `TestName` functions in `*_test.go`. Prefer focused tests for config validation, storage behavior, auth, PII scrubbing, RAG cache behavior, and error paths. Run `GOTOOLCHAIN=auto go vet ./...` and `GOTOOLCHAIN=auto go test ./... -v` before backend submissions. For frontend changes, add `*.test.tsx` or `*.test.ts` coverage where practical and run `npm test`, `npm audit --audit-level=moderate`, `npm run typecheck`, and `npm run build`.

## Commit & Pull Request Guidelines

The repo history uses concise conventional commits, for example `feat(phase-5): ...`, `docs: ...`, and `chore: ...`. Keep subjects imperative and scoped. Do not add `Co-Authored-By` lines.

Pull requests should include a short summary, tests/builds run, configuration changes, and screenshots for UI changes. Link related issues or plan items when applicable.

## Security & Configuration Tips

Do not commit API keys, `.env`, SQLite databases, binaries, or generated `web/dist` assets. Configure secrets with environment variables such as `SYS_SENTIENT_GEMINI_API_KEY` and `SYS_SENTIENT_SERVER_API_KEY`. Keep PII scrubbing enabled when sending logs to Gemini.
