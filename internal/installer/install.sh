#!/bin/sh
# SysSentient installer.
#
# Fetches the release binary for this machine, verifies it against the
# published checksums, and optionally enrols with a server. Written for POSIX
# sh so it runs on Alpine and busybox, not just bash.
#
#   curl -fsSL https://<server>/install.sh | sh -s -- --token <token>
#
# Every step prints what it is doing. An installer that works silently is
# indistinguishable from one that has hung.
set -eu

REPO="gysosin/SysSentient"
SERVER=""
TOKEN=""
VERSION="latest"
PREFIX="${PREFIX:-/usr/local/bin}"
INSTALL_SERVICE="yes"

usage() {
    cat <<'USAGE'
Usage: install.sh [--server URL --token TOKEN] [options]

  --server URL     SysSentient server to enrol with
  --token TOKEN    single-use join token from Settings -> Devices
  --version TAG    release to install (default: latest)
  --prefix DIR     install directory (default: /usr/local/bin)
  --no-service     install the binary without registering a service
  --help           show this message

Without --server and --token the binary is installed but not enrolled.
USAGE
}

while [ $# -gt 0 ]; do
    case "$1" in
        --server) SERVER="${2:?--server needs a URL}"; shift 2 ;;
        --token)  TOKEN="${2:?--token needs a value}"; shift 2 ;;
        --version) VERSION="${2:?--version needs a tag}"; shift 2 ;;
        --prefix) PREFIX="${2:?--prefix needs a directory}"; shift 2 ;;
        --no-service) INSTALL_SERVICE="no"; shift ;;
        --help|-h) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

die() { echo "error: $*" >&2; exit 1; }
say() { echo "==> $*"; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }
need uname
need tar

# curl or wget, whichever the machine has. Assuming curl fails on minimal
# Debian images, which ship wget instead.
if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1" -o "$2"; }
    fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$2" "$1"; }
    fetch_stdout() { wget -qO- "$1"; }
else
    die "neither curl nor wget is available"
fi

case "$(uname -s)" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *) die "unsupported operating system: $(uname -s). Windows users: use install.ps1" ;;
esac

case "$(uname -m)" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) die "unsupported architecture: $(uname -m)" ;;
esac

say "detected ${OS}/${ARCH}"

if [ "$VERSION" = "latest" ]; then
    say "resolving the latest release"
    # /releases/latest excludes pre-releases and 404s when every release is
    # one -- which is exactly the case for a project still in beta. Fall back
    # to the newest release of any kind rather than failing on a repo that
    # plainly has releases.
    VERSION=$(fetch_stdout "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1) || true
    if [ -z "$VERSION" ]; then
        say "no stable release; using the newest pre-release"
        VERSION=$(fetch_stdout "https://api.github.com/repos/${REPO}/releases" \
            | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
    fi
    [ -n "$VERSION" ] || die "could not determine a release to install"
fi
say "installing ${VERSION}"

NUMERIC_VERSION="${VERSION#v}"
ARCHIVE="sys-sentient_${NUMERIC_VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${VERSION}"

WORKDIR=$(mktemp -d)
# Cleaned up on any exit, including a failure part-way through, so a retry
# does not accumulate half-downloaded archives.
trap 'rm -rf "$WORKDIR"' EXIT INT TERM

say "downloading ${ARCHIVE}"
fetch "${BASE}/${ARCHIVE}" "${WORKDIR}/${ARCHIVE}" || die "could not download ${ARCHIVE}"

say "verifying the checksum"
if fetch "${BASE}/checksums.txt" "${WORKDIR}/checksums.txt"; then
    EXPECTED=$(grep " ${ARCHIVE}\$" "${WORKDIR}/checksums.txt" | awk '{print $1}')
    [ -n "$EXPECTED" ] || die "no checksum published for ${ARCHIVE}"

    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL=$(sha256sum "${WORKDIR}/${ARCHIVE}" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        ACTUAL=$(shasum -a 256 "${WORKDIR}/${ARCHIVE}" | awk '{print $1}')
    else
        die "no sha256 tool available; refusing to install an unverified binary"
    fi

    # A mismatch means the download is not what was published. Refuse rather
    # than warn: this script may be piped straight into a shell as root.
    [ "$EXPECTED" = "$ACTUAL" ] || die "checksum mismatch for ${ARCHIVE}"
    say "checksum verified"
else
    die "could not download checksums.txt; refusing to install an unverified binary"
fi

tar -xzf "${WORKDIR}/${ARCHIVE}" -C "$WORKDIR"

# v0.1.0-beta.1 shipped the binary as sys-daemon; it was renamed to match the
# product afterwards. Accept either so the installer works against the release
# that is actually published today as well as the ones that follow.
BINARY="${WORKDIR}/sys-sentient"
[ -f "$BINARY" ] || BINARY="${WORKDIR}/sys-daemon"
[ -f "$BINARY" ] || die "the archive contained neither sys-sentient nor sys-daemon"

SUDO=""
if [ ! -w "$PREFIX" ]; then
    command -v sudo >/dev/null 2>&1 || die "$PREFIX is not writable and sudo is unavailable"
    SUDO="sudo"
fi

say "installing to ${PREFIX}/sys-sentient"
$SUDO mkdir -p "$PREFIX"
$SUDO install -m 0755 "$BINARY" "${PREFIX}/sys-sentient"

if [ -z "$TOKEN" ] || [ -z "$SERVER" ]; then
    say "installed. To enrol this machine later:"
    echo "    ${PREFIX}/sys-sentient agent join --server <url> --token <token> --install-service"
    exit 0
fi

# Releases before enrolment existed have no `agent join` subcommand, so the
# arguments fall through to the daemon's own flag parsing and it starts a
# server instead of enrolling -- which then fails on a port already in use and
# looks like a networking problem. Refuse clearly instead.
case "$VERSION" in
    v0.1.0-beta.1)
        echo "error: ${VERSION} predates agent enrolment and cannot join a server." >&2
        echo "       The binary is installed at ${PREFIX}/sys-sentient." >&2
        echo "       Install a newer release: install.sh --version <tag> --server ... --token ..." >&2
        exit 1
        ;;
esac

say "enrolling with ${SERVER}"
JOIN_ARGS="agent join --server ${SERVER} --token ${TOKEN}"
[ "$INSTALL_SERVICE" = "yes" ] && JOIN_ARGS="${JOIN_ARGS} --install-service"

# Enrolment writes to /etc when run as root and to the user's config directory
# otherwise; the binary decides, so the same command works either way.
# shellcheck disable=SC2086
$SUDO "${PREFIX}/sys-sentient" $JOIN_ARGS

say "done"
