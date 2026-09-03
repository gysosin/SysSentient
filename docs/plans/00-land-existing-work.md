# 00 — Land the existing work on `beta`

| | |
|---|---|
| **Phase** | 0 — Land |
| **Depends on** | nothing |
| **Status** | not started |

## Why

99 uncommitted working-tree entries, including whole packages (`internal/agent`,
`internal/alerting`, `internal/auth`, `internal/hostid`, `internal/logging`,
`internal/version`, `web/pages`, `web/components/ui`) and every metadata file.

`README.md` on GitHub links to `CHANGELOG.md`, `SECURITY.md` and
`CONTRIBUTING.md` — all three are untracked, so all three are 404s today.
`.github/workflows/release.yml` cannot fire on a tag that does not exist on a
branch containing it.

## Scope

- Branch `beta` off `main` (currently 0 ahead / 0 behind `origin/main`).
- Commit in conventional-commit slices, never one blob.
- Repair `CHANGELOG.md`: duplicated `Added`/`Changed`/`Fixed` headings, an
  orphaned bullet block with no heading, and a `Security` note that contradicts
  the `Removed` section.
- Repair `web/README.md`: it still instructs users to set
  `VITE_SYS_SENTIENT_API_KEY`, the exact variable removed for leaking the key
  into the published bundle.
- Push and open a PR to `main`.

## Acceptance criteria

- `beta` exists on the remote and a PR is open against `main`.
- CI is green on the PR.
- `git check-ignore -v config.yaml` reports `.gitignore:67`; no Gemini key
  appears anywhere in the diff.
- Every link in `README.md` resolves on the remote.

## Verification

```bash
git check-ignore -v config.yaml
git log --oneline origin/main..beta
gh pr view --web
```
Then open the PR's Files-changed tab and confirm no `config.yaml`, no `*.db`,
no `web/dist`.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
