# 12 — Devices screen — enrol an agent from the UI

| | |
|---|---|
| **Phase** | 3 — Fleet |
| **Depends on** | 10, 11 |
| **Status** | **done** — Settings → Devices enrols, lists and revokes |

## Why

Connecting a new machine is currently a hand-edited config file. It should be:
click "Add device", copy one line, run it on the target, watch the host appear.

## Scope

- A **Devices** screen: agent list with version, last-seen, health and revoke.
- "Add device" issues a join token and shows a copyable one-liner.
- Live enrolment status; per-host detail view.
- Empty state that explains what an agent is and what it sends.

## Acceptance criteria

- A machine is enrolled end to end without touching a config file.
- Revoke works from the UI and the agent stops being accepted.
- Viewers cannot issue or revoke tokens; admins can.

## Verification

```bash
cd web && npm test
```
Plus a manual two-machine enrolment with no file editing.

---

Every shard must also pass the project gate before it is pushed:

```bash
GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build
```
