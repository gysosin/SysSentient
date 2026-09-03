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

Enrol the machine rather than hand-writing its config. On the server, open
**Settings → Devices**, name the machine, and press *Generate command*. Run the
command it gives you on the machine itself:

```bash
sys-sentient agent join --server https://monitor.example.com --token <token> --install-service
```

That exchanges the single-use token for a credential belonging to this machine
alone, writes `agent.yaml` with mode `0600`, and registers a service so the
agent survives logout and reboot. The device appears on the Devices screen once
it reports.

Drop `--install-service` to enrol without registering anything; the command
then prints how to start the agent by hand.

Useful flags:

| Flag | Purpose |
|---|---|
| `--config <path>` | Where to write the config. Defaults to `/etc/sys-sentient/agent.yaml` as root, otherwise the user config directory. |
| `--ca-cert <path>` | Trust a private CA for the server connection. |
| `--insecure-skip-verify` | Disable TLS verification. Last resort; it makes the connection trivially interceptable. |
| `--force` | Overwrite an existing config. Refused by default, so re-running the command cannot silently orphan a working credential. |
| `--install-service` | Register a service so the agent restarts on boot. Uses a per-user service when not run as root. |

Join tokens are single use and expire after an hour by default (24 hours is the
maximum). Only the hash is stored, so the token is shown once at creation and
cannot be retrieved afterwards — generate a new one instead.

The written config looks like this:

```yaml
mode: agent
agent:
  server_url: "https://monitor.example.com"
  key: "<this machine's own credential>"
  spool_path: /var/lib/sys-sentient/spool.jsonl
```

An agent opens no database and serves no dashboard. It collects on the poll
interval, batches, and pushes.

### Keeping it running

`--install-service` covers this during enrolment. To add, inspect or remove the
service later:

```bash
sys-sentient service install --config /etc/sys-sentient/agent.yaml
sys-sentient service status
sys-sentient service uninstall
```

It writes a systemd unit on Linux, a launchd job on macOS, and registers with
the service manager on Windows. Add `--user` for a per-user service that needs
no root — the right choice for an agent enrolled to a config in your home
directory.

`service status` distinguishes a service that is starting from one that is
crash-looping, and names the command to read its logs:

```
Installed: /etc/systemd/system/sys-sentient.service
State:     stopped (failed: exit-code, 10 restarts -- check the logs: journalctl -u sys-sentient -n 20)
```

### Removing a machine

Press **Revoke** on the Devices screen. The credential stops working
immediately; the row stays, marked revoked, so a machine that disappears from
the fleet has a visible reason rather than looking like a bug.

A revoked agent keeps collecting locally and says so plainly in its log:

```
ERROR this agent has been revoked; it will keep collecting but the server will
not accept its data. Re-enrol with `sys-sentient agent join` using a new token.
```

To bring it back, generate a new token and re-run `agent join --force`.

### Writing config by hand

Still supported, and `server.agent_key` still authenticates agents that use it
— an existing fleet keeps working across the upgrade that introduced per-agent
credentials, and can migrate one machine at a time. Per-agent credentials are
tried first, the shared key second.

Set `server.public_url` if the server sits behind a reverse proxy; without it
the enrolment command is built from the request's `Host`, which is the proxy's
own name.

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

A host is registered on first contact with no approval step. A join token is
therefore worth protecting for its lifetime: anyone holding one can enrol a
machine and report under any host id. They are single-use and short-lived to
keep that window small.

`server.agent_key`, if set, remains a shared fleet-wide secret with no
per-agent rotation or revocation. Prefer enrolment; keep the shared key only
for machines not yet migrated, and give it the handling you would give a
database password.

Put the server behind TLS either way. Over plain HTTP the join token and the
credential it returns both cross the network in the clear — `agent join` warns
when you do this, but it cannot make it safe.
