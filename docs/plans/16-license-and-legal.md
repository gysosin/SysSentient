# 16 — Apache-2.0 and third-party notices

| | |
|---|---|
| **Phase** | 5 — Docs |
| **Depends on** | nothing |
| **Status** | not started |

## Why

There is **no LICENSE file**, which means the project is "all rights reserved"
by default: nobody may legally redistribute or modify it. This blocks the
GitHub Pages site, the release, and the whole "someone installs this" goal.

All dependencies are MIT/BSD/Apache with no copyleft, so the choice is free.

## Scope

- Add `LICENSE` (Apache-2.0) and `NOTICE`.
- Generate `docs/THIRD_PARTY.md` from `go.mod` and `web/package.json`.
- Add the SPDX identifier to `package.json` and the README badge.
- Remove the "not yet licensed" warnings from `README.md` and `CHANGELOG.md`.

## Acceptance criteria

- GitHub detects and displays the licence.
- Every dependency licence is listed and compatible.

## Verification

```bash
go run github.com/google/go-licenses@latest report ./... 2>/dev/null | head
gh repo view --json licenseInfo
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
