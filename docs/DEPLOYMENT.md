# Deployment

## One machine

The default. `mode: all-in-one` collects locally, stores, and serves the
dashboard. Nothing else to configure.

## A fleet

Split the roles. One **server** stores and serves; each machine runs an
**agent** that collects and pushes.

### Server

```yaml
mode: server
server:
  port: 8080
  agent_key: "<a long random string>"
```

The server does not collect locally in this mode. It waits for agents to push
to `/api/ingest`.

### Agent

```yaml
mode: agent
agent:
  server_url: "https://monitor.example.com"
  key: "<the same agent_key>"
  spool_path: /var/lib/sys-sentient/spool
  batch_size: 60
```

An agent opens no database and serves no dashboard. It collects on the poll
interval, batches, and pushes.

### Surviving a network partition

Samples that cannot be delivered go to an on-disk spool, bounded at 5,000
entries, oldest dropped on overflow. When the server returns, the backlog is
flushed.

Alerts are evaluated against the newest sample per host only, so a replayed
backlog does not fire an alert for every historical sample it contains.

### TLS

Agents push over HTTPS. Point `agent.ca_cert_path` at your CA if the server
uses a private one. `agent.insecure_skip_verify` exists as a last resort and
warns loudly; it defeats the point of using HTTPS at all.

## Current limits

There is **one shared agent key for the whole fleet**. It cannot be rotated per
agent, and a host is registered on first contact with no approval step, so
anyone holding the key can register any host id.

Per-agent credentials, join tokens, revocation, and enrolling a machine from
the dashboard are the next work — see
[plans/10-agent-join-tokens.md](plans/10-agent-join-tokens.md) through
[plans/12-devices-screen.md](plans/12-devices-screen.md).

Until then, treat `agent_key` as a shared secret: give it the handling you
would give a database password, and put the server behind TLS.
