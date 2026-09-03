# 08 — Backup and export the records

| | |
|---|---|
| **Phase** | 2 — Performance |
| **Depends on** | 06, 07 |
| **Status** | not started |

## Why

There is no documented way to back up or move the data. Copying a live WAL
database with `cp` produces a corrupt file, which is the mistake people make by
default.

> **Prior art:** four deleted `feature/export-*` branches implemented CSV
> and JSON export for metrics, logs and insights against the old console.
> See [21](21-harvest-from-stale-branches.md) — the shapes are worth
> reading before designing this.

## Scope

- A `sys-sentient backup` command using SQLite's online backup API.
- CSV and JSON export endpoints, scoped by host and time range.
- Document restore, and document that a plain file copy is unsafe.

## Acceptance criteria

- A backup taken while the daemon is writing restores cleanly and passes
  `PRAGMA integrity_check`.
- Export round-trips without losing precision.

## Verification

```bash
./sys-daemon backup --out /tmp/backup.db   # while the daemon is running
sqlite3 /tmp/backup.db "PRAGMA integrity_check;"
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
