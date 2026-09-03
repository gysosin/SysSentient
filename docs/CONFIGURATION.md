# Configuration

Copy [`config.yaml.example`](../config.yaml.example) to `config.yaml`, or use
environment variables: every key maps to `SYS_SENTIENT_` plus the path in caps
with dots as underscores, so `alerting.webhook_url` becomes
`SYS_SENTIENT_ALERTING_WEBHOOK_URL`.

Configuration is read from `./config.yaml` and `/etc/sys-sentient/config.yaml`.

## Changeable without a restart

These are editable from **Settings → Configuration** in the dashboard, or
through `PATCH /api/settings`, and take effect immediately.

| Key | Default | Description |
|---|---|---|
| `collector.poll_interval_seconds` | 2 | How often the machine is sampled |
| `database.metrics_retention_hours` | 24 | How long full-resolution samples are kept |
| `database.minute_rollup_days` | 30 | Per-minute averages and peaks |
| `database.five_minute_rollup_days` | 365 | The coarse tier; data leaves the system here |
| `logging.level` | info | `debug` adds a line per sample |

Everything else requires a restart. A port or database path cannot change under
a running process without reopening sockets or re-deriving state.

## Server

| Key | Default | Description |
|---|---|---|
| `server.port` | 8080 | Web server port |
| `server.api_key` | – | Machine token for scripts, sent as `X-API-Key`. Browser users sign in instead |
| `server.agent_key` | – | Token remote agents authenticate with. Falls back to `api_key` |
| `server.insecure` | false | Disables authentication entirely. Warned on every start |
| `server.allowed_origins` | localhost:8080, :3000, :5173 | CORS and WebSocket origin allowlist |

## Authentication

| Key | Default | Description |
|---|---|---|
| `auth.session_idle_hours` | 24 | Sign out after this much inactivity |
| `auth.session_max_days` | 30 | Absolute session lifetime |
| `auth.login_rate_per_minute` | 5 | Password attempts per client IP; each costs an argon2 hash |

## Collection and storage

| Key | Default | Description |
|---|---|---|
| `collector.top_processes` | 10 | Processes recorded per sample |
| `collector.host_id` | – | Override the derived machine identifier |
| `database.path` | sys-sentient.db | Database location |
| `database.insights_retention_hours` | 168 | AI insight and alert history retention |

Retention is tiered rather than a single cut-off; see
[PERFORMANCE.md](PERFORMANCE.md#storage) for why and what it costs.

## Mode

| Key | Default | Description |
|---|---|---|
| `mode` | all-in-one | `all-in-one`, `server` or `agent` |
| `agent.server_url` | – | Required in agent mode |
| `agent.key` | – | Required in agent mode |
| `agent.spool_path` | – | On-disk buffer for network outages |
| `agent.batch_size` | 60 | Samples per push |
| `agent.ca_cert_path` | – | Custom CA for the server's certificate |
| `agent.insecure_skip_verify` | false | Last resort; warned about |

See [DEPLOYMENT.md](DEPLOYMENT.md) for how the modes fit together.

## Alerting

| Key | Default | Description |
|---|---|---|
| `alerting.enabled` | true | Evaluate alert rules |
| `alerting.webhook_url` | – | Receives raw alert JSON per transition |
| `alerting.slack_webhook_url` | – | Slack incoming webhook |

With neither channel set, alerts still appear in the dashboard and the daemon
warns at start-up that nobody will be told.

## AI and privacy

| Key | Default | Description |
|---|---|---|
| `gemini.api_key` | – | AI is disabled entirely when empty |
| `gemini.model_name` | gemini-2.5-flash-lite | Model |
| `gemini.max_daily_cost` | 1.0 | Hard USD cap per UTC day; 0 disables |
| `privacy.mask_ips` | true | Redaction before anything reaches Gemini |
| `privacy.mask_emails` | true | |
| `privacy.mask_usernames` | true | |

Nothing leaves the machine unless `gemini.api_key` is set. See
[PRIVACY.md](PRIVACY.md).
