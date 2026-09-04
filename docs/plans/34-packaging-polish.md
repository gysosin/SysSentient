# 34 — One package, for people who don't live in a terminal

| | |
|---|---|
| **Phase** | 7 — Distribution |
| **Depends on** | 04, 29 |
| **Status** | **done** — Homebrew, Scoop, and a first run you can't miss |

## Why

The install paths worked but assumed a comfortable operator. The one thing a
new user must do — read a setup token and open a URL — was logged as a single
structured line among a dozen others at startup. Fine for someone tailing a
journal; useless for someone who just double-clicked a binary.

A Homebrew tap and a Scoop manifest were scoped in plan 04 and never built.

## What changed

- **Homebrew cask and Scoop manifest** in `.goreleaser.yaml`. The cask strips
  the Gatekeeper quarantine attribute on install — an unsigned binary otherwise
  fails its first run with a dialog rather than a useful error.
- **A first-run notice** that cannot be missed:

  ```
  ┌────────────────────────────────────────────────────────────────────┐
  │ SysSentient is running. One step left.                             │
  │                                                                    │
  │ Open:  http://localhost:8080/setup                                 │
  │ Token: K_OqnuEczOrpjGUtxMIhn5tCxJfXEoPWBqN                         │
  │                                                                    │
  │ The token is shown once and creates the first administrator.       │
  └────────────────────────────────────────────────────────────────────┘
  ```

  Printed **only to a real terminal** — in a journal or container log the box
  drawing is noise, and the structured line already carries the token. The
  browser is opened at the setup page, best effort: a headless server has none,
  and failing to open one is not a reason to refuse to start.

- **The token is deliberately not in the URL.** A secret in a URL ends up in
  browser history; the operator pastes it from the terminal.

## Notes

Neither the cask nor the manifest starts a service on install, for the same
reason the Linux packages do not: the first run mints a one-time setup token,
and a service launched by the package manager emits it before anyone is
watching.

The formatting is separated from the terminal check so it is actually testable
— a test cannot hand the printer a real terminal, and a test that therefore
asserts nothing is worse than no test. Four tests cover the content, the box
alignment, the silence on a non-terminal, and that the token stays out of the
URL.
