#!/usr/bin/env bash
#
# Publishes docs/wiki/ to the repository's GitHub wiki.
#
# GitHub has no API for creating a wiki's first page: the REST API returns 404
# for /wiki, and the wiki's git repository does not exist until a page has been
# saved through the web interface. Cloning or pushing before that fails with
# "Repository not found" whether or not you are authenticated. So the first run
# of this script tells you where to click; every run after that just syncs.
set -euo pipefail

REPO="${1:-gysosin/SysSentient}"
# Overridable so the sync path can be exercised against a local repository;
# GitHub will not let anyone test it against a wiki that does not exist yet.
WIKI_URL="${WIKI_URL:-https://github.com/${REPO}.wiki.git}"
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/docs/wiki"

if [ ! -d "$SOURCE" ]; then
    echo "error: no such directory: $SOURCE" >&2
    exit 1
fi

# gh supplies credentials when it is installed and logged in; without it, git's
# own helpers are used instead.
git_wiki() {
    if command -v gh >/dev/null 2>&1; then
        git -c credential.helper='!gh auth git-credential' "$@"
    else
        git "$@"
    fi
}

# The wiki is a separate repository, so a clone of it inherits neither this
# repository's identity nor necessarily a global one -- git then refuses to
# commit with "Author identity unknown". Carry the identity across explicitly,
# falling back to the authenticated GitHub account.
AUTHOR_NAME="$(git -C "$(dirname "$SOURCE")/.." config user.name 2>/dev/null || true)"
AUTHOR_EMAIL="$(git -C "$(dirname "$SOURCE")/.." config user.email 2>/dev/null || true)"
if [ -z "$AUTHOR_NAME" ] && command -v gh >/dev/null 2>&1; then
    AUTHOR_NAME="$(gh api user --jq .login 2>/dev/null || true)"
fi
if [ -z "$AUTHOR_EMAIL" ] && [ -n "${AUTHOR_NAME:-}" ]; then
    AUTHOR_EMAIL="${AUTHOR_NAME}@users.noreply.github.com"
fi
if [ -z "$AUTHOR_NAME" ] || [ -z "$AUTHOR_EMAIL" ]; then
    echo "error: no git identity available; set user.name and user.email" >&2
    exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

if ! git_wiki clone --quiet "$WIKI_URL" "$WORK/wiki" 2>/dev/null; then
    cat >&2 <<MSG
The wiki has no pages yet, so its git repository does not exist.

GitHub only creates it when the first page is saved from the browser; there is
no API for this, and an authenticated clone or push fails the same way.

  1. Open https://github.com/${REPO}/wiki
  2. Click "Create the first page" and save it. Any content will do -- this
     script overwrites it.
  3. Run this script again.

MSG
    exit 2
fi

# README.md documents the directory for anyone reading the repository; it is
# not a wiki page. An earlier [A-Z]*.md glob was commented as excluding it and
# did not -- README starts with a capital letter like everything else.
PAGES=()
for page in "$SOURCE"/*.md; do
    [ "$(basename "$page")" = "README.md" ] && continue
    PAGES+=("$page")
    cp "$page" "$WORK/wiki/"
done
if [ ${#PAGES[@]} -eq 0 ]; then
    echo "error: no pages found in $SOURCE" >&2
    exit 1
fi

cd "$WORK/wiki"
if git diff --quiet && git diff --cached --quiet && [ -z "$(git status --porcelain)" ]; then
    echo "Wiki is already up to date."
    exit 0
fi

git add -A
git -c "user.name=$AUTHOR_NAME" -c "user.email=$AUTHOR_EMAIL" \
    commit --quiet -m "docs(wiki): sync from docs/wiki"
git_wiki push --quiet origin HEAD
echo "Published ${#PAGES[@]} pages to https://github.com/${REPO}/wiki"
