# 03 — Add Windows and macOS platform seams

| | |
|---|---|
| **Phase** | 1 — Packaging |
| **Depends on** | 01 |
| **Status** | **done** — logs and host id have per-OS implementations |

## Why

Metric collection is already cross-platform through gopsutil, but two packages
are Linux-only and there are currently **zero build tags in the repo**:

- `internal/logs/reader.go` shells out to `journalctl`, `dmesg` and
  `tail /var/log/syslog`. These fail soft, so on Windows you get empty logs —
  and AI analysis quality degrades to nothing.
- `internal/hostid/hostid.go` reads `/etc/machine-id` and
  `/var/lib/dbus/machine-id`, falling back to a hostname hash. On Windows a
  rename silently starts a new history.

## Scope

- Split both packages into `_linux.go` / `_windows.go` / `_darwin.go`.
- Windows: Event Log for logs, `MachineGuid` from the registry for host id.
- macOS: `log show` for logs, `IOPlatformUUID` for host id.
- Keep the soft-fail behaviour; never panic on an unsupported platform.

## Acceptance criteria

- Each platform has a real log source and a stable machine identifier.
- `go vet` passes for all three `GOOS` values.
- Host id is stable across a hostname change on every platform.

## Verification

```bash
for os in linux windows darwin; do GOOS=$os CGO_ENABLED=0 go vet ./... ; done
```
Plus a manual run on a Windows box confirming logs are non-empty.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
