# Wiki source

These pages are the GitHub wiki's content, kept here so they are reviewed and
versioned with the code rather than edited only in the wiki's own UI.

They are deliberately *not* a second copy of `docs/`. `docs/` is the reference —
every configuration key, the architecture, the measured numbers. These pages
answer the questions that come up in use, and link to `docs/` for the detail.

## Publishing

GitHub does not create a wiki's git repository until its first page exists, and
that first page can only be created through the web UI — the API can enable the
wiki (already done) but not initialise it.

So, once:

1. Open https://github.com/gysosin/SysSentient/wiki and press **Create the
   first page**. Any content will do; the next step overwrites it.

Then, to publish these pages (and again whenever they change):

```bash
git clone https://github.com/gysosin/SysSentient.wiki.git /tmp/ss-wiki
cp docs/wiki/[A-Z]*.md /tmp/ss-wiki/
cd /tmp/ss-wiki && git add -A && git commit -m "docs(wiki): sync from docs/wiki" && git push
```

Page titles come from filenames: `Adding-a-machine.md` becomes "Adding a
machine". `Home.md` is the landing page. This README is excluded by the `[A-Z]*`
glob, which is why it is named in lower case.
