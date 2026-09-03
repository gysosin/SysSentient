# 10 — Per-agent identity and join tokens

| | |
|---|---|
| **Phase** | 3 — Fleet |
| **Depends on** | 09 |
| **Status** | not started |

## Why

There is exactly **one shared agent key for the whole fleet**
(`internal/server/server.go:75` falls back to the dashboard key). No per-agent
identity, no rotation, no revocation — rotating the key means reconfiguring and
restarting every agent simultaneously. A host is auto-registered on first push,
so any holder of that key can register any `host_id`.

`internal/auth` is human-users only; its `SetupToken` is unrelated and is `nil`
once a user exists.

## Scope

- Join-token issue/redeem endpoints, reusing `internal/auth/token.go`'s
  `NewToken` / `HashToken`.
- A per-agent credential minted on redemption; store a hash, never the token.
- Agent list with version and last-seen; revoke.
- Relax `DisallowUnknownFields` on ingest — it currently makes an older server
  reject a newer agent with a 400.

## Acceptance criteria

- A token is single-use, expires, and can be revoked.
- Revoking one agent does not affect any other.
- The old shared key still works behind a deprecation flag for one release.

## Verification

```bash
GOTOOLCHAIN=auto go test ./internal/auth/... ./internal/server/... -race
```
Plus: enrol an agent, revoke it, and confirm its next push is rejected.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
