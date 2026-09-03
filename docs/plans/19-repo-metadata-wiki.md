# 19 — Repo metadata, templates and wiki

| | |
|---|---|
| **Phase** | 5 — Docs |
| **Depends on** | 16, 18 |
| **Status** | **partly done** — README, templates, CoC; repo settings and wiki are manual |

## Why

The repo has no description, no topics, no homepage, no social preview, no
issue or PR templates, no `CODE_OF_CONDUCT.md`, no `CODEOWNERS`. There are also
39 stale `origin/feature/*` branches cluttering the branch list.

## Scope

- Set description, topics and homepage URL; upload a social preview image.
- Add `CODE_OF_CONDUCT.md`, issue templates (bug / feature / security),
  a PR template, and `CODEOWNERS`.
- Rewrite `README.md` with badges and a screenshot.
- Seed the wiki: install, configuration, troubleshooting, FAQ.
- Delete the 39 stale feature branches.

## Acceptance criteria

- The repo front page reads like a product, not a scratch directory.
- A new visitor can find install instructions within one click.

## Verification

```bash
gh repo view --json description,repositoryTopics,homepageUrl
git branch -r | wc -l
```

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
