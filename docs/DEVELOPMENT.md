# Development

## Prerequisites

Go (the version in `go.mod`) and Node 22. No C toolchain: the SQLite driver is
pure Go.

## The gate

Everything CI runs:

```bash
make verify
```

Or individually:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
make lint                       # golangci-lint, pinned to the CI version
make vuln                       # govulncheck
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
docker build --pull -t sys-sentient:verify .
```

Run `make lint` locally before pushing. It and `govulncheck` have each caught
real failures that a clean `go test` missed.

## Running it

```bash
make build && ./sys-daemon
```

The dashboard must be built first: it is embedded at compile time. For frontend
work, `cd web && npm run dev` serves on port 3000 and proxies the API — a real
`web/dist` on disk takes precedence over the embedded copy, so changes are
picked up without recompiling the daemon.

## Things that will bite you

**Tailwind v4 does not read `tailwind.config.js`** without the `@config`
directive at the top of `web/index.css`. Without it the entire palette compiles
to nothing, silently. Guarded by `styles.config.test.ts` and
`npm run verify:css`, which asserts the tokens reach the built bundle — the
declaration existing is not the same as it working.

**There is a font-size floor.** `typography.test.ts` fails the build on any
`text-[Npx]` below 12px. The console has drifted into unreadable type twice.

**Two transports.** Metrics arrive over WebSocket when connected and REST
polling otherwise. A change to one usually needs the other.

**The PII scrubber is the egress gate.** Anything added to the Gemini prompt
must go through it.

**Benchmarks need `-count=3`.** A single short run is dominated by scheduling
noise; see [PERFORMANCE.md](PERFORMANCE.md).

## Conventions

Conventional commits with imperative subjects: `feat(...)`, `fix(...)`,
`sec(...)`, `perf(...)`, `chore(...)`, `docs(...)`.

Update `README.md`, `CHANGELOG.md` and the relevant `docs/` page in the same
commit as the change they describe.

One shard of work per branch; see [plans/](plans/README.md).

## Do not commit

API keys, `.env`, `config.yaml`, SQLite databases, binaries, or generated
`web/dist` assets. `web/dist/.gitkeep` is the one tracked exception, because
the embed directive needs the directory to exist.
