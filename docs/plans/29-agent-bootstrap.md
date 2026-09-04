# 29 — Let the server hand out the agent

| | |
|---|---|
| **Phase** | 3 — Fleet |
| **Depends on** | 11, 12 |
| **Status** | **done** — one pasted command, from nothing to enrolled |

## Why

The Devices screen said *"Install SysSentient on the machine, then run the
command below on it"* and offered nothing to install it with. The server served
**no binaries, installers or scripts**, and there was no `install.sh` or
`install.ps1` in the repository. You had to already have the binary to be told
how to enrol it.

## What changed

- `internal/installer/install.sh` and `install.ps1`, embedded in the binary and
  served at `/install.sh` and `/install.ps1`. They live beside their embed
  rather than in a top-level `scripts/` directory because Go can only embed at
  or below the embedding package, and two copies of a script that must stay
  identical is a trap: the one you edit is never the one that ships.
- The Devices screen offers three commands — Linux/macOS, Windows, and
  "already installed" — instead of assuming the binary is there.
- The scripts **refuse to install an unverified binary**. They are piped
  straight into a shell, often as root, so a missing or mismatched checksum is
  a hard failure rather than a warning.

## Three bugs found by running it

**`/releases/latest` 404s when every release is a pre-release.** GitHub excludes
them, which is exactly the case for a project in beta — so the one-liner failed
on a repository that plainly has releases. Both scripts now fall back to the
newest release of any kind.

**The published archive contains `sys-daemon`, not `sys-sentient`.** The rename
came after v0.1.0-beta.1 was tagged. The scripts accept either.

**v0.1.0-beta.1 has no `agent join` subcommand.** It predates enrolment, so the
arguments fell through to the daemon's own flag parsing and it started a
*server* — which then failed on a port already in use and looked like a
networking problem. The scripts now refuse with a message naming the cause.

## Acceptance

A real install from the real release, into an empty directory:

```
==> detected linux/amd64
==> no stable release; using the newest pre-release
==> installing v0.1.0-beta.1
==> downloading sys-sentient_0.1.0-beta.1_linux_amd64.tar.gz
==> verifying the checksum
==> checksum verified
==> installing to …/sys-sentient

$ sys-sentient --version
sys-sentient 0.1.0-beta.1 (3e4fba3) built 2026-09-03T17:38:34Z go1.25.13 linux/amd64
```

With `--server` and `--token` against the current release it refuses, correctly,
because that release cannot enrol.

## Still needed

**A release newer than v0.1.0-beta.1.** The installer is complete and the
enrolment path works from source, but the only published binary predates
`agent join`, so the end-to-end bootstrap cannot finish against it. Cutting the
next tag closes this.
