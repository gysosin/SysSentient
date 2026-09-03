# 17 — Restructure `docs/` and de-duplicate

| | |
|---|---|
| **Phase** | 5 — Docs |
| **Depends on** | 16 |
| **Status** | **done** — one home per topic, all links verified |

## Why

`docs/` held only two working-notes files, one of them 3,623 lines of agent
scratch. Meanwhile the real reference content is scattered and duplicated:
Configuration appears in both `README.md` and `QUICK_START.md`; build commands
appear in `README.md`, `QUICK_START.md`, `AGENTS.md`, `CONTRIBUTING.md` **and**
the `Makefile`. The auth model is described in four places.

`AGENTS.md` is stale — it says `npm test` uses Node's test runner (it is Vitest)
and omits half the packages.

## Scope

- Author `docs/`: `ARCHITECTURE.md`, `INSTALL.md`, `USER_GUIDE.md`,
  `CONFIGURATION.md`, `DEPLOYMENT.md`, `PERFORMANCE.md`, `PRIVACY.md`,
  `RELEASING.md`, `DEVELOPMENT.md`, `ROADMAP.md`.
- Give each duplicated topic exactly one home; everything else links to it.
- Normalise `QUICK_START.md`'s emoji headings to house style, or fold it into
  `docs/USER_GUIDE.md`.
- Refresh `AGENTS.md` and `CLAUDE.md`.

## Acceptance criteria

- No topic is documented in two places.
- Every internal link resolves.
- `docs/README.md` is a real index.

## Verification

```bash
find docs -name '*.md' | xargs grep -ohE '\]\([^)h][^)]*\)' | tr -d ']()' | sort -u
```
Check each path exists.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
