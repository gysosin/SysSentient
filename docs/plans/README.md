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
| 21 | [Harvest from stale branches](21-harvest-from-stale-branches.md) | reference |
| [22](22-service-install.md) | `service install` for systemd, launchd and Windows | done |
| [23](23-server-mode-maintenance.md) | Make server mode a first-class citizen | done |
| [24](24-time-range-queries.md) | Time-range query engine | done |
| [27](27-process-accuracy.md) | Make processes true | done |
| [30](30-insight-history.md) | Insight history | done |
| [25](25-chart-timerange.md) | Chart drill-down and a time picker | done |
| [26](26-export-ui.md) | Export from the UI | done |
| [31](31-notifications-and-rules.md) | Notification centre and rule control | done |
| [29](29-agent-bootstrap.md) | Let the server hand out the agent | done |
| [28](28-hosts-screen.md) | Hosts as a first-class screen | done |
| [32](32-ai-assistant.md) | Agentic AI assistant | done |
| [33](33-mcp-server.md) | MCP server | done |
| [34](34-packaging-polish.md) | One package, for people who don't live in a terminal | done |
| [35](35-archiving.md) | Archiving, so the disk does not fill | done |
| [36](36-pause-when-hidden.md) | Stop the dashboard working while nobody is looking | done |

## Working agreement

One shard at a time. Each is fully tested before it is pushed, and
`README.md` / `CHANGELOG.md` / `docs/` are updated **in the same commit** as the
change they describe.

`archive/` holds superseded working notes, kept only so inbound links resolve.
