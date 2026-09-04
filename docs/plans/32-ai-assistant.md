# 32 — Agentic AI assistant

| | |
|---|---|
| **Phase** | 6 — Intelligence |
| **Depends on** | 24, 27, 30 |
| **Status** | **done** — asks the database, not the void |

## Why

The AI was strictly one-shot: a single `GenerateContent` call over one snapshot,
no chat session, no tools, no follow-up. It could describe the moment it was
handed and nothing else — so "why was it slow at 3pm?" was unanswerable.

## What changed

- A **read-only tool surface** (`internal/ai/tools.go`): query a metrics window,
  list top processes at an instant, list hosts, read recent alerts, read logs,
  read past analyses. Defined as an interface so the same surface can be
  exposed over MCP without either caller reaching through the AI package to
  the database.
- A **tool-calling loop** (`internal/ai/chat.go`). The model looks something
  up, then looks up more based on what it found. Bounded at ten calls per
  question — a model that keeps calling tools without answering would
  otherwise spend the operator's budget in a loop.
- `POST /api/chat`, admin-only and sharing the analysis rate limiter, because a
  chat turn makes several model calls and is the more expensive of the two.
- A chat panel on Insights that **shows which tools produced each answer**. An
  answer you cannot check is a guess with better formatting.

## Boundaries

Every tool is read-only, and the system prompt says the assistant cannot change
the system. Suggested commands remain suggestions the operator runs themselves,
matching the existing "commands are not validated by the daemon" boundary.

Tool results pass through the same PII scrubbing as the one-shot analysis —
usernames, emails, IPs and home paths — because these strings go to a model on
somebody else's hardware. The daily spend cap covers chat, and every model call
in the loop is charged against it.

## Acceptance

Against the real API, on live data:

```
Q: Which hosts are reporting, and what has CPU been doing on the busiest one
   over the last hour?
   tools: list_hosts → query_metrics
   "The busiest host is fedora. Over the last hour, CPU averaged 50.9% and
    peaked at 99.9% at 2026-09-04T13:28:43Z."

Q: CPU spiked recently. Find when, and tell me exactly which process caused it.
   tools: query_metrics → list_hosts → top_processes
   "The CPU spike to 99.9% at 2026-09-04T13:05:25Z on the fedora host was
    caused by the `compile` process (PID 1213004)…"
```

Both answers name real values and real timestamps, drawn from the tools rather
than invented.
