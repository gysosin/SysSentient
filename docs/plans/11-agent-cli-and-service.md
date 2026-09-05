# 11 — `agent join` CLI and service install

| | |
|---|---|
| **Phase** | 3 — Fleet |
| **Depends on** | 10 |
| **Status** | **done** — `agent join` writes a 0600 config; spool made append-only |

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

## What shipped, and what did not

`agent join` ships: it enrols the machine, writes a `0600` config that is
guaranteed to load through the daemon's own reader, and prints the start
command. The spool fix that this shard also called for ships too — see below.

**Service installation from the CLI did not ship *in this shard*** — it
shipped later in [22-service-install.md](22-service-install.md), which added
`sys-sentient service install|uninstall|status` for systemd, launchd and the
Windows service manager. The paragraph below describes the state at the time
this shard closed.

**Original note.** `agent join` prints the
command to run rather than registering a systemd unit, a Windows service or a
launchd job itself. The packages already install a service unit
(`packaging/`), so the gap only affects someone who installed from a raw
binary. Registering a service means writing to `/etc/systemd/system` or the
Windows SCM as root, which deserves its own shard rather than being folded into
an enrolment command that otherwise needs no privileges.

## Spool: measured

The spool rewrote its entire file on every append. Against a full
5,000-sample buffer, on this machine:

| | ns/op | per append |
|---|---|---|
| Before | 230,464,629 | 230 ms |
| After (plain append) | 27,079 | 0.027 ms |
| After (amortised, including compaction) | 475,864 | 0.48 ms |

`BenchmarkSpoolAppendWhenFull`, `-benchtime 3000x`. 230 ms is 11% of a
2-second poll interval, paid on every sample for the whole duration of the
outage the spool exists to survive.
