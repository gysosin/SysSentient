#!/usr/bin/env python3
"""Checks the public website before it is published.

A broken link or a missing meta tag on a project's front page is the kind of
thing nobody notices for months, because nobody re-reads their own landing
page. This runs in CI on every change to site/.

Deliberately dependency-free: it runs on a stock GitHub runner with no install
step, so it cannot fall out of date with a requirements file nobody updates.
"""

from __future__ import annotations

import pathlib
import re
import sys
from html.parser import HTMLParser

SITE = pathlib.Path(__file__).resolve().parent.parent / "site"

REQUIRED_FILES = ["index.html", "404.html", "robots.txt", "sitemap.xml"]

# Without these a shared link renders as a bare URL, and search results show
# whatever text the crawler happened to pick.
REQUIRED_META = [
    ("name", "description"),
    ("name", "viewport"),
    ("property", "og:title"),
    ("property", "og:description"),
    ("property", "og:url"),
]


class Extractor(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.links: list[str] = []
        self.meta: list[tuple[str, str]] = []
        self.title = ""
        self._in_title = False
        self.lang = ""

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        d = {k: (v or "") for k, v in attrs}
        if tag == "a" and "href" in d:
            self.links.append(d["href"])
        elif tag == "meta":
            for key in ("name", "property"):
                if key in d:
                    self.meta.append((key, d[key]))
        elif tag == "title":
            self._in_title = True
        elif tag == "html":
            self.lang = d.get("lang", "")

    def handle_endtag(self, tag: str) -> None:
        if tag == "title":
            self._in_title = False

    def handle_data(self, data: str) -> None:
        if self._in_title:
            self.title += data


def main() -> int:
    problems: list[str] = []

    for name in REQUIRED_FILES:
        if not (SITE / name).is_file():
            problems.append(f"missing {name}")

    index = SITE / "index.html"
    if not index.is_file():
        print("verify_site: site/index.html is missing", file=sys.stderr)
        return 1

    html = index.read_text(encoding="utf-8")
    page = Extractor()
    page.feed(html)

    if not page.title.strip():
        problems.append("index.html has no <title>")
    if not page.lang:
        problems.append("<html> has no lang attribute (screen readers need it)")

    present = set(page.meta)
    for key, value in REQUIRED_META:
        if (key, value) not in present:
            problems.append(f"index.html is missing <meta {key}=\"{value}\">")

    # Relative links must point at something that exists; absolute ones are not
    # this script's business, but an obviously malformed one is.
    for href in page.links:
        if href.startswith(("http://", "https://", "mailto:", "#")):
            if href.startswith(("http://", "https://")) and " " in href:
                problems.append(f"malformed URL: {href}")
            continue
        target = href.split("#")[0].split("?")[0].lstrip("/")
        if not target:
            continue
        if not (SITE / target).exists():
            problems.append(f"broken relative link: {href}")

    # An unclosed <pre> or <code> silently swallows the rest of the page.
    for tag in ("pre", "code", "table", "main", "footer"):
        opened = len(re.findall(rf"<{tag}[\s>]", html))
        closed = len(re.findall(rf"</{tag}>", html))
        if opened != closed:
            problems.append(f"unbalanced <{tag}>: {opened} opened, {closed} closed")

    if problems:
        print("verify_site: FAILED\n", file=sys.stderr)
        for p in problems:
            print(f"  - {p}", file=sys.stderr)
        return 1

    print(f"verify_site: OK — {len(REQUIRED_FILES)} files, {len(page.links)} links checked")
    return 0


if __name__ == "__main__":
    sys.exit(main())
