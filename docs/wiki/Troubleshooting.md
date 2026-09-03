# Troubleshooting

## The dashboard is blank

The binary embeds the dashboard, so this is almost always a browser cache from
an older build. Hard-reload. If you are running from source, `web/dist` must
exist — run `npm run build` in `web/`.

## I lost the setup token

It is logged once, at startup, when no user account exists. If no account was
created yet, restart the daemon and it logs a fresh one. If an account does
exist, use the sign-in page instead.

## An agent is not appearing

Check its log first — it names the cause:

| Log line | Meaning |
|---|---|
| `revoked` | The credential was withdrawn. Generate a new token and re-run `agent join --force`. |
| `did not recognise this agent's credential` | Wrong or missing `agent.key`. Re-enrol. |
| `push failed, samples remain spooled` | Transient: the server is unreachable. Samples are buffered and retried. |

## `datetime()` returns NULL, or timestamps look wrong

Fixed in current builds. The pure-Go SQLite driver wrote `time.Time` in Go's
own `String()` format, which SQLite's date functions cannot parse. Every
binding site now uses an explicit layout. If you have a database written by an
affected build, the migration repairs it in place.

## The database file never shrinks

SQLite does not return freed pages to the filesystem on its own. Compaction
runs periodically; `sys-sentient --backup <path>` also writes a fully compacted
copy.

## High CPU

Raise `collector.poll_interval_seconds` — it is the dominant factor, and it is
editable in **Settings → Configuration** without a restart. Idle cost is 0.78%
of one core at a 2-second interval.

If you are measuring the daemon's own usage, match the process by exact name
(`pgrep -x sys-sentient`). A `pgrep -f` pattern also matches the shell you type
it in, which produces confident readings for the wrong process.

## Reporting a bug

Open an issue at
https://github.com/gysosin/SysSentient/issues with the output of
`sys-sentient --version` and the relevant log lines.
