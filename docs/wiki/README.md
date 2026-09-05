# Wiki source

These pages are the GitHub wiki's content, kept here so they are reviewed and
versioned with the code rather than edited only in the wiki's own UI.

They are deliberately *not* a second copy of `docs/`. `docs/` is the reference —
every configuration key, the architecture, the measured numbers. These pages
answer the questions that come up in use, and link to `docs/` for the detail.

## Publishing

```bash
./tool/publish_wiki.sh
```

That is the whole thing, on every run after the first.

**The first run needs two clicks from a human, and cannot be automated.**
GitHub creates a wiki's git repository only when its first page is saved from
the web interface. There is no API for it: `/repos/{owner}/{repo}/wiki` returns
404, and cloning or pushing to `<repo>.wiki.git` fails with "Repository not
found" whether or not you are authenticated, and whether or not `has_wiki` is
true. The script detects this and prints where to click.

So, once:

1. Open <https://github.com/gysosin/SysSentient/wiki>
2. Click **Create the first page** and save it. Any content will do — the
   script overwrites it.

Then `./tool/publish_wiki.sh` publishes every page here and is a no-op when
nothing has changed.

Page titles come from filenames: `Adding-a-machine.md` becomes "Adding a
machine", and `Home.md` is the landing page. This README is skipped by name.
