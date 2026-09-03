# 14 — Split Settings into modules

| | |
|---|---|
| **Phase** | 4 — Console |
| **Depends on** | 00 |
| **Status** | **done** — tabbed, URL-addressable sections |

## Why

All six Settings sections — Daemon, Configuration, Privacy, Integrations, Users,
Account — render in one flat two-column grid, so change-password sits beside API
endpoint documentation. There is no grouping and no sense of place.

## Scope

- Tabbed sections under `web/pages/settings/`: General, Collection,
  Privacy & AI, Integrations, Devices, Users, Account.
- Each tab its own component and its own route, so it is linkable.
- Redesign the change-password and user-management forms properly.
- Use the shadcn `Tabs` wrapper (Radix is already a dependency).

## Acceptance criteria

- Each section is independently addressable by URL.
- Admin-only sections stay gated; viewers do not see them.
- The existing `#account` deep link from the user menu still works.

## Verification

```bash
cd web && npm test && npm run typecheck
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
