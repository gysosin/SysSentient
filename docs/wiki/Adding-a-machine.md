# Adding a machine

One server can collect from many machines. Each one runs the same binary in
agent mode.

## Enrol it

Install SysSentient on the new machine, then in the server's dashboard open
**Settings → Devices**, name the machine, and press *Generate command*. Run
what it gives you on that machine:

```bash
sys-sentient agent join --server https://monitor.example.com --token <token>
```

That exchanges the token for a credential belonging to that machine alone,
writes a config readable only by its owner, and prints how to start it. The
device appears on the Devices screen once it reports, with its version and when
it was last seen.

## About the token

Single use, and it expires after an hour by default. Only its hash is stored,
so it is shown once at creation and cannot be retrieved afterwards — generate a
new one instead. Expired, spent and never-issued tokens all fail identically,
so the endpoint cannot be used to discover which tokens exist.

## Removing a machine

Press **Revoke**. The credential stops working immediately. The row stays,
marked revoked, so a machine that disappears from the fleet has a visible
reason rather than looking like a bug.

A revoked agent keeps collecting locally and says so in its log:

```
ERROR this agent has been revoked; it will keep collecting but the server will
not accept its data. Re-enrol with `sys-sentient agent join` using a new token.
```

To bring it back, generate a new token and re-run `agent join --force`.

## Behind a reverse proxy

Set `server.public_url`. Without it the enrolment command is built from the
request's `Host` header, which behind a proxy is the proxy's own name.

## During a network outage

Samples that cannot be delivered are buffered on disk, bounded at 5,000
entries with the oldest dropped on overflow. When the server returns, the
backlog is flushed. Alerts are evaluated against the newest sample per host
only, so a replayed backlog does not fire an alert for every historical sample
it contains.
