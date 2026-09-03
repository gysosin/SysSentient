# Roadmap

From working prototype to installable product. Each phase is sharded into
[`docs/plans/`](plans/README.md) so it can be picked up, tested and pushed one
piece at a time.

**Decisions:** Apache-2.0 · work lands on `beta` with a PR to `main` ·
packaging first.

## Phases

| Phase | Goal | Shards |
|---|---|---|
| **0 — Land** | Get the existing work committed and reviewable | 00 |
| **1 — Packaging** | Install it on Linux, Windows or macOS and it runs | 01–04 |
| **2 — Performance** | Small footprint, and keep the records | 05–09 |
| **3 — Fleet** | Enrol a remote agent from the UI | 10–12 |
| **4 — Console** | Readable, full-width, drillable, modular | 13–15, 20 |
| **5 — Docs** | Licence, docs site, and a repo that reads like a product | 16–19 |

## Measured baselines

Taken on a Fedora workstation, 8 cores, at the default 2s poll. These are the
numbers the performance work is measured against — not estimates.

| Metric | Today | Target |
|---|---|---|
| Idle CPU | **4.1%** | < 1% |
| Resident memory | 34 MB | ≤ 40 MB |
| Storage per host per day | **~171 MiB** (4.3 KB/row × 41,553 rows) | < 20 MB |
| Metrics retention | 24 h, then hard `DELETE` | 24 h raw → 30 d @ 1 min → 1 y @ 5 min |
| Cross-compile targets | 1 (linux/amd64, needs CGO) | 6 (linux/windows/darwin × amd64/arm64) |

## Known blockers

Three findings gate the packaging work and are addressed first:

1. **CGO.** `CGO_ENABLED=0 go build ./cmd/daemon` fails — `mattn/go-sqlite3`
   needs a C toolchain, so there is no static binary and no non-Linux target.
   The coupling is four lines. → [01](plans/01-drop-cgo.md)
2. **The binary is not relocatable.** The dashboard is served from `./web/dist`
   relative to the working directory, which is why the systemd unit pins
   `WorkingDirectory=/opt/sys-sentient`. No package can ship that.
   → [02](plans/02-embed-dashboard.md)
3. **No LICENSE.** The project is "all rights reserved" by default, so nobody
   may legally redistribute it. → [16](plans/16-license-and-legal.md)

## Working agreement

One shard at a time, fully tested before it is pushed, with `README.md`,
`CHANGELOG.md` and `docs/` updated in the same commit as the change they
describe.

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
docker build --pull -t sys-sentient:verify .
```
