# 01 — Replace `mattn/go-sqlite3` with `modernc.org/sqlite`

| | |
|---|---|
| **Phase** | 1 — Packaging |
| **Depends on** | 00 |
| **Status** | **done** — merged, all six targets build |

## Why

`CGO_ENABLED=0 go build ./cmd/daemon` fails with `undefined: sqlite3.Error`.
CGO means no static binary, and cross-compiling to Windows, macOS or arm64
needs a full C toolchain per target. This is the single blocker for
"installable on any OS". Both `Dockerfile` and `.github/workflows/release.yml`
already carry comments naming this as the fix.

The coupling is only four lines, so this is contained.

## Scope

- `internal/storage/sqlite.go:11` — swap the blank import.
- `internal/storage/sqlite.go:19` — driver name `sqlite3` → `sqlite`.
- `internal/storage/sqlite.go` `sqliteDSN` — rewrite
  `?_journal_mode=WAL&_busy_timeout=5000&…` into modernc's
  `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&…` form.
- `internal/storage/users.go:9,75-76` — replace the `sqlite3.Error` /
  `ErrConstraintUnique` assertion with a driver-agnostic unique-violation check.
- Benchmark write throughput before and after and record both numbers.

## Acceptance criteria

- `CGO_ENABLED=0 go build ./cmd/daemon` succeeds.
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64` and `GOOS=darwin GOARCH=arm64`
  both build.
- All storage tests pass unchanged; the unique-violation path keeps its test.
- Write benchmark regression is documented and accepted (~2× slower is fine at
  a 2s interval).

## Verification

```bash
CGO_ENABLED=0 go build -o /dev/null ./cmd/daemon
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/daemon
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o /dev/null ./cmd/daemon
GOTOOLCHAIN=auto go test ./internal/storage/... -race
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
