# 33 — MCP server

| | |
|---|---|
| **Phase** | 6 — Intelligence |
| **Depends on** | 32 |
| **Status** | **done** — the same tools, from outside the browser |

## Why

The assistant's tools are useful outside the dashboard. Exposing them over MCP
lets Claude Desktop — or anything else that speaks the protocol — answer the
same questions about your fleet.

## What changed

- `/mcp`, a streamable-HTTP MCP server built on the official Go SDK, offering
  the **same six read-only tools** the in-dashboard assistant uses. One surface,
  so a question answerable in the console is answerable outside it without a
  second implementation to keep in step.
- Authenticated **exactly like every other API route** — a session cookie or an
  API key. An MCP client therefore uses a credential the operator already
  understands and can already revoke, rather than a second, parallel notion of
  access.
- A tool failure is returned as tool content with `isError`, not as a transport
  error: the client's model can read it and try something else, where a
  protocol error aborts the whole call.

## Connecting

Point an MCP client at `http://<server>/mcp` with an `X-API-Key` header set to
`server.api_key`.

## Acceptance

Against the running daemon:

```
unauthenticated POST /mcp                     → 401
initialize                                    → protocolVersion 2025-06-18,
                                                 serverInfo sys-sentient
tools/list   → list_hosts, query_metrics, recent_alerts,
               recent_insights, recent_logs, top_processes

tools/call list_hosts
  2 host(s) reporting:
    id 5d3733f8b7af49c706e3195df58d6cc2  hostname fedora  last seen 14:34:36Z
    id build-server-demo-0001            hostname fedora  last seen 12:28:00Z

tools/call query_metrics (last hour)
  Window …13:34:36Z to …14:34:36Z on fedora, 378 samples.
  CPU: avg 49.1%, peak 99.9% at 2026-09-04T14:16:41Z.
  At the end of the window: load 8.19, 629 processes running.

tools/call query_metrics {"from":"yesterday"}
  error: from must be RFC3339, got "yesterday"
```
