# 19 — Repo metadata, templates and wiki

| | |
|---|---|
| **Phase** | 5 — Docs |
| **Depends on** | 16, 18 |
| **Status** | **done** — metadata set, wiki written; one manual click to publish |

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


## Outcome

Repository description, homepage and 12 topics are set. Issue and PR templates,
`CODE_OF_CONDUCT.md` and `CODEOWNERS` are in the repository. The site is live at
https://gysosin.github.io/SysSentient/.

Wiki pages — Home, FAQ, Adding a machine, Troubleshooting — are written and
version-controlled in [`docs/wiki/`](../wiki/), so they are reviewed with the
code rather than edited only in the wiki's own UI.

**One step cannot be automated.** GitHub does not create a wiki's git repository
until its first page exists, and that page can only be created through the web
UI: the API enables the wiki (done — `has_wiki` is now true) but will not
initialise it, and `git push` to `SysSentient.wiki.git` fails with "Repository
not found" until it is. `docs/wiki/README.md` has the one click and the sync
command.
