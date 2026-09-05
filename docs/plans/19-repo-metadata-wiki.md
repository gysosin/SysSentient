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


## Publishing, revisited

`tool/publish_wiki.sh` now does the sync in one command and reports what to do
when the wiki has no pages yet.

Every automated route to that first page was tried and none exists: the REST
API has no wiki endpoint (404), `has_wiki` is true yet `<repo>.wiki.git` is not
provisioned, and clone and push both fail with "Repository not found"
authenticated with `repo` scope as well as anonymously. GitHub creates the wiki
repository only when a page is saved from the browser.

Two bugs surfaced while testing the script against a stand-in repository rather
than assuming it worked: it committed with no git identity (a wiki clone
inherits neither this repository's config nor necessarily a global one), and
its `[A-Z]*.md` glob published `README.md` as a wiki page despite a comment
claiming it excluded it.
