# 22 — `service install` for systemd, launchd and Windows

| | |
|---|---|
| **Phase** | 3 — Fleet |
| **Depends on** | 11 |
| **Status** | **done** — systemd verified live; launchd and Windows compile-only |

## Why

`agent join` enrols a machine and prints a command to run. Nothing keeps the
agent running after that: close the shell and it stops, reboot and it never
comes back. The packages install a unit, but only for a server-mode install at
the default config path — an agent enrolled to `~/.config/sys-sentient/agent.yaml`
has nothing.

This was deferred out of shard 11 because registering a service means writing to
`/etc/systemd/system` or the Windows SCM as root, which does not belong folded
into an enrolment command that otherwise needs no privileges.

## Scope

- `sys-sentient service install|uninstall|status`, and `--install-service` on
  `agent join` so enrolling and persisting is one command when the operator
  wants it.
- systemd on Linux, both system units and `--user` units, so an unprivileged
  agent can persist without root.
- launchd on macOS.
- Windows SCM, which additionally requires the daemon to answer service control
  requests — without that the SCM starts it and then marks it failed.
- Refuse to overwrite an existing unit unless `--force`, matching `agent join`.

## Acceptance

- A generated unit starts, reports active, survives `systemctl restart`, and is
  removed cleanly by `service uninstall`.
- Verified against real systemd, not just by inspecting the generated file.
- `service status` reports honestly when no service is installed.


## Outcome

`sys-sentient service install|uninstall|status`, plus `--install-service` on
`agent join`. The enrolment command shown in the dashboard now includes it, so
the default path leaves a machine that keeps reporting after a reboot.

Verified against real systemd (user scope), full lifecycle: not installed →
install → active and serving → survives `systemctl restart` → uninstall →
systemd reports `not-found`, port closed → uninstalling again is a no-op.

Two bugs found by running it rather than reading the generated unit:

- `PrivateTmp=true` gives the service its own `/tmp`, so a binary or config
  under `/tmp` is invisible to it. The unit failed with `203/EXEC` "No such
  file or directory" for a file that plainly existed. The directive is now
  omitted, with a comment saying why, when a path would be hidden by it — and
  kept for every normal install.
- `service status` reported `activating` for a service systemd was restarting
  in a loop, which reads as "starting up" rather than "broken". It now reads
  `SubState` and `NRestarts` and reports
  `failed: exit-code, 10 restarts -- check the logs: journalctl --user -u sys-sentient -n 20`.

## Not verified

launchd and the Windows service manager compile and are exercised by unit tests
for the parts that are pure logic, but neither has been run on its own platform
— there is no macOS or Windows machine here, and CI only compiles. The Windows
path additionally required a service control handler in `cmd/daemon`: without
one the SCM starts the process and then marks the service failed, because a
plain console binary never answers the dispatcher.
