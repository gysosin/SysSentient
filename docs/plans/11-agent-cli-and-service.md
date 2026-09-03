# 11 — `agent join` CLI and service install

| | |
|---|---|
| **Phase** | 3 — Fleet |
| **Depends on** | 10 |
| **Status** | not started |

## Why

`internal/config/config.go` hard-requires both `agent.server_url` and
`agent.key` at boot, so an agent cannot start and wait to be enrolled. There are
only two CLI flags — `--version` and `--config` — so an installer has no way to
say "join this server with this token".

`internal/agent/spool.go` also rewrites the entire spool file on **every single
sample**; with a full 5000-sample buffer that is re-serialising ~20 MB every 2
seconds during an outage.

## Scope

- `sys-sentient agent join --server <url> --token <t>`: redeem, write config,
  install the platform service, start it.
- Allow an unenrolled start state instead of failing validation at boot.
- Add `--mode`, `--server`, `--token` flags.
- Replace the spool's full rewrite with an append-only log plus compaction.
- Document `mode`, `agent.*` and `server.agent_key` in `config.yaml.example` —
  none of them appear there today.

## Acceptance criteria

- One command on a fresh machine enrols it and starts pushing.
- A 10-minute network partition loses no samples.
- Spool append is O(1), verified by benchmark.

## Verification

```bash
GOTOOLCHAIN=auto go test ./internal/agent/... -race -bench=.
```
Plus a real two-machine test with the server stopped for 10 minutes.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
