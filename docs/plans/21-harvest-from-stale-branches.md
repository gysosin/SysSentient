# 21 — Features harvested from the stale feature branches

| | |
|---|---|
| **Phase** | 4 — Console |
| **Depends on** | nothing (reference document) |
| **Status** | harvested; branches deleted |

## Why

38 `feature/*` branches sat on the remote, all forked from the pre-rewrite
`main` (2026-05-09) and carrying 39 unique commits between them. They could not
be merged: of the 39 files they touched, **27 no longer exist** after the
console rewrite — they build on `web/components/ProcessList.tsx`, the old
`App.tsx`, and their own components, all of which the rewrite replaced. Merging
would have resurrected deleted files and produced a console that does not
build.

The *ideas* were worth keeping, so they are recorded here and folded into the
shards below. The branches were then deleted.

## Already delivered by the console rewrite

No further work needed:

- `feat(processes): add search filter` — the Processes filter box
- `feat(processes): add resource sorting` — sortable columns with `aria-sort`
- `feat(logs): add keyword search` — the Logs search box
- `feat(logs): add clear filters action` — level toggles clear the same way
- `feat(insights): add action command copy` — copy button per suggested command
- `feat(insights): add empty action state` — "No analysis yet" state
- `feat(insights): add action safety summary` — the model-says-destructive /
  low-risk badges
- `feat(ai): add manual scan status flow` — idle / running / failed / succeeded

## Folded into existing shards

| Harvested commit | Shard |
|---|---|
| `feat(metrics): add csv export` | [08 — backup and export](08-backup-and-export.md) |
| `feat(logs): add csv export`, `add json export` | [08](08-backup-and-export.md) |
| `feat(ai): add insight json export` | [08](08-backup-and-export.md) |
| `feat(metrics): add chart range switcher` | [20 — drill-down](20-drilldown-widgets.md) — the range selector it needs |
| `feat(logs): add severity explorer`, `add facility breakdown`, `add facility quick filters` | [20](20-drilldown-widgets.md) — a Logs drill-down |
| `feat(logs): add entry inspector` | [20](20-drilldown-widgets.md) |
| `feat(logs): add tail pause control` | Delivered as the global freeze control; per-pane pause folded into [20](20-drilldown-widgets.md) |
| `feat(processes): add safe action preview` | [10 — agent tokens](10-agent-join-tokens.md) permissions model applies; the process-signal action itself is out of scope until the daemon can send signals |

## Still unclaimed — worth their own shards

- **`feat(dashboard): add saved views`** — persist a filter/layout combination
  and switch between them. Genuinely new capability; nothing in the current plan
  covers it.
- **`feat(dashboard): add system health command center`** — a single
  whole-fleet summary surface. Overlaps the Overview hero, but the fleet
  dimension is not covered.
- **`feat(dashboard): add critical alerts rail`** — a persistent rail of firing
  alerts across every screen, rather than only the nav badge.
- **`feat(ai): add insight history timeline`** — insight history over time.
  `GET /api/insights` already returns history; nothing renders it as a
  timeline.
- The `feat(insights): add …` cluster (priority badges, severity summary,
  status filter, recency labels, result limit, density toggle, trend strip,
  restricted-action counter, safe-action ratio, latest-incident banner) all
  assume **many** stored insights. They become worthwhile once insight history
  is rendered — track them behind the timeline above.

## Recovering a deleted branch

The commits are unreachable but not immediately gone. Until git prunes them:

```bash
git fetch origin '+refs/heads/*:refs/remotes/origin/*'
git log --oneline --all --reflog | grep '<subject>'
```

The full list of harvested commit subjects is preserved above. Every deleted
branch tip is recorded below, so any one can be restored with:

```bash
git fetch origin <sha>
git branch <name> <sha>
```

| Tip | Branch |
|---|---|
| `3925e7c32b` | feature/ai-insight-action-copy |
| `7ba54a00b9` | feature/ai-insight-action-inspector |
| `84e3d4af69` | feature/ai-insight-action-load-indicator |
| `903093b823` | feature/ai-insight-action-safety-summary |
| `0ee33058e9` | feature/ai-insight-clear-filters |
| `6a26b5afd1` | feature/ai-insight-density-toggle |
| `72a45390c5` | feature/ai-insight-empty-action-state |
| `40079589b6` | feature/ai-insight-history-timeline |
| `0f990565e9` | feature/ai-insight-keyword-search |
| `1014cfc3fc` | feature/ai-insight-latest-incident-banner |
| `fb2833e037` | feature/ai-insight-priority-badges |
| `46da9c9622` | feature/ai-insight-priority-summary |
| `9e1971e15c` | feature/ai-insight-recency-labels |
| `654252343b` | feature/ai-insight-restricted-action-counter |
| `cf901c7dc6` | feature/ai-insight-result-limit |
| `3da2678272` | feature/ai-insight-safe-action-ratio |
| `31fcdbb1c4` | feature/ai-insight-severity-summary |
| `697a57e6fc` | feature/ai-insight-status-filter |
| `5c71a23883` | feature/ai-insight-status-trend-strip |
| `a737120806` | feature/critical-alerts-rail |
| `1b426a115b` | feature/export-insights-json |
| `81d0915f9b` | feature/export-logs-csv |
| `29796fa9ad` | feature/export-logs-json |
| `ac7ae7676b` | feature/export-metrics-csv |
| `2e0ed317de` | feature/log-clear-filters |
| `080abdba80` | feature/log-entry-inspector |
| `bafbefcd98` | feature/log-facility-breakdown |
| `83455e7028` | feature/log-facility-quick-filters |
| `01f1cd6f0f` | feature/log-keyword-search |
| `36781e0b65` | feature/log-pause-tail-control |
| `2d9f824e09` | feature/log-severity-explorer |
| `df4b8ea429` | feature/manual-ai-analysis-status-flow |
| `a0229d220a` | feature/metric-time-range-switcher |
| `75cb956dd8` | feature/process-resource-sorting |
| `d32367a891` | feature/process-safe-action-preview |
| `dbbdb060a0` | feature/process-search-filter |
| `6ca7d53c82` | feature/saved-dashboard-views |
| `e42aca8e10` | feature/system-health-command-center |
