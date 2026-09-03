# 02 — Embed the dashboard so the binary is relocatable

| | |
|---|---|
| **Phase** | 1 — Packaging |
| **Depends on** | 01 |
| **Status** | **done** — binary serves the UI from any cwd |

## Why

`internal/server/server.go:165` serves the UI from `./web/dist`, relative to the
working directory. That is precisely why `sys-sentient.service` hardcodes
`WorkingDirectory=/opt/sys-sentient`.

No `.deb`, `.rpm`, `.msi` or tarball can ship a binary that only works from one
directory. Packaging cannot start until this is fixed.

## Scope

- `go:embed` the built `web/dist` into the binary, behind the existing
  `staticHandler`.
- Keep a development escape hatch (env var or build tag) that serves from disk,
  so `npm run dev` still works.
- Drop `WorkingDirectory` from the systemd unit.
- Make the build order explicit in the `Makefile`: `web` must precede `daemon`.

## Acceptance criteria

- `./sys-daemon` serves the dashboard from any working directory.
- `cd /tmp && /path/to/sys-daemon` renders the UI, not a 404.
- The systemd unit no longer pins a working directory.

## Verification

```bash
make build
cd /tmp && /path/to/sys-daemon &
curl -fsS http://localhost:8080/ | grep -q '<div id="root">' && echo "UI served from an arbitrary cwd"
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
