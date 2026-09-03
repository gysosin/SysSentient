# Implementation plans

The roadmap is sharded so each piece can be picked up, tested and pushed on its
own. Work them roughly in order; dependencies are stated in each file.

| # | Shard | Phase |
|---|---|---|
| 00 | [Land the existing work](00-land-existing-work.md) | 0 — Land |
| 01 | [Drop CGO](01-drop-cgo.md) | 1 — Packaging |
| 02 | [Embed the dashboard](02-embed-dashboard.md) | 1 — Packaging |
| 03 | [Platform seams](03-platform-seams.md) | 1 — Packaging |
| 04 | [GoReleaser packaging](04-goreleaser-packaging.md) | 1 — Packaging |
| 05 | [Collector hot path](05-collector-hot-path.md) | 2 — Performance |
| 06 | [Tiered retention](06-tiered-retention.md) | 2 — Performance |
| 07 | [Storage footprint](07-storage-footprint.md) | 2 — Performance |
| 08 | [Backup and export](08-backup-and-export.md) | 2 — Performance |
| 09 | [Fix UpsertHost](09-fix-upserthost.md) | 2 — Performance |
| 10 | [Agent join tokens](10-agent-join-tokens.md) | 3 — Fleet |
| 11 | [Agent CLI and service](11-agent-cli-and-service.md) | 3 — Fleet |
| 12 | [Devices screen](12-devices-screen.md) | 3 — Fleet |
| 13 | [Typography and layout](13-typography-and-layout.md) | 4 — Console |
| 14 | [Modular settings](14-modular-settings.md) | 4 — Console |
| 15 | [Runtime settings](15-runtime-settings.md) | 4 — Console |
| 16 | [Licence and legal](16-license-and-legal.md) | 5 — Docs |
| 17 | [Docs restructure](17-docs-restructure.md) | 5 — Docs |
| 18 | [Pages site](18-pages-site.md) | 5 — Docs |
| 19 | [Repo metadata and wiki](19-repo-metadata-wiki.md) | 5 — Docs |
| 20 | [Drill-down widgets](20-drilldown-widgets.md) | 4 — Console |

## Working agreement

One shard at a time. Each is fully tested before it is pushed, and
`README.md` / `CHANGELOG.md` / `docs/` are updated **in the same commit** as the
change they describe.

`archive/` holds superseded working notes, kept only so inbound links resolve.
