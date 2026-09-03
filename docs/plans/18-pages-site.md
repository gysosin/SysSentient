# 18 — GitHub Pages site

| | |
|---|---|
| **Phase** | 5 — Docs |
| **Depends on** | 16, 17 |
| **Status** | **done** — deploys behind a verification gate |

## Why

There is no docs site, no `site/` directory, no Pages workflow — a clean slate.
`gysosin/droidpier` is the reference: a static `site/` deployed by
`.github/workflows/pages.yml` behind a `tool/verify_site.py` gate.

## Scope

- `site/`: landing page, screenshots, feature list, install instructions, and
  the AI-diagnosis differentiator. Plus `404.html`, `robots.txt`, `sitemap.xml`.
- `.github/workflows/pages.yml` deploying on pushes that touch `site/`.
- A link verifier so a broken link fails the build.

## Acceptance criteria

- `https://gysosin.github.io/SysSentient` is live.
- Lighthouse: accessibility and SEO both ≥ 95.
- No broken links.

## Verification

```bash
python3 tool/verify_site.py
```
Then open the deployed URL and run Lighthouse.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
