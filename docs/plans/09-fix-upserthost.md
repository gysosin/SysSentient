# 09 — Register the local host in all-in-one mode

| | |
|---|---|
| **Phase** | 2 — Performance |
| **Depends on** | 00 |
| **Status** | not started |

## Why

`UpsertHost` is called only from `internal/server/ingest.go:100`, which runs
only when a remote agent pushes. The all-in-one collector loop never calls it.

Verified against the live database: **0 rows in `hosts` against 41,553 rows in
`metrics`.** The Settings UI hides this by rendering `${hosts.length || 1}`.
Fleet features in Phase 3 build on this table, so it has to be correct first.

## Scope

- Call `UpsertHost` from the all-in-one collect loop.
- Remove the `|| 1` fallback in the UI once the table is populated.
- Add a regression test asserting a host row exists after the first sample.

## Acceptance criteria

- `GET /api/hosts` returns the local host on a single-node install.
- The host selector and fleet count reflect reality rather than a fallback.

## Verification

```bash
./sys-daemon & sleep 5
curl -fsS localhost:8080/api/hosts | jq 'length'   # must be >= 1
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
