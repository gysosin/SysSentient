# 15 — Editable settings without a restart

| | |
|---|---|
| **Phase** | 4 — Console |
| **Depends on** | 14 |
| **Status** | **done** — interval, retention and log level apply live |

## Why

Config is read once at boot. There is no `viper.WatchConfig` and no config write
API among the 25 routes, so changing the poll interval — the setting you
specifically asked for — requires editing a file and restarting the daemon.

## Scope

- A config write API with validation and an audit entry.
- Hot-reload the collector ticker, retention window and log level without a
  restart.
- Make it obvious in the UI which settings are live and which still need a
  restart.

## Acceptance criteria

- Changing the poll interval from the UI takes effect within one cycle.
- An invalid value is rejected with a useful message and nothing is written.
- Only admins can write config.

## Verification

```bash
curl -X PATCH localhost:8080/api/config -d '{"collector":{"poll_interval_seconds":5}}'
```
Then confirm sample spacing changes without a restart.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
