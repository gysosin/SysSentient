# SysSentient

SysSentient is an intelligent system monitor that uses Google Gemini AI to analyze system metrics and logs.

## Architecture

- **Daemon (`sys-daemon`)**: Go application that collects metrics, stores them in SQLite, and exposes a JSON API. It also handles AI analysis.
- **Web UI**: React application that visualizes metrics and provides an interface for AI insights.

## Prerequisites

- Go 1.25.10+ or Go toolchain auto-download enabled
- Node.js 22+
- Google Gemini API Key

## Building

### 1. Build the Web Interface
```bash
cd web
npm install
npm run build
cd ..
```

### 2. Build the Daemon
```bash
go build -o sys-daemon ./cmd/daemon
```

### Container Image
```bash
docker build -t sys-sentient .
docker run --rm -p 8080:8080 \
  -v sys-sentient-data:/var/lib/sys-sentient \
  -e SYS_SENTIENT_SERVER_API_KEY="your_dashboard_key" \
  -e SYS_SENTIENT_GEMINI_API_KEY="your_api_key" \
  sys-sentient
```

The container writes SQLite data to `/var/lib/sys-sentient`; keep that path on
a named volume for persistent metrics and insight history.

## Running

Ensure `web/dist` exists (from step 1).

```bash
export SYS_SENTIENT_GEMINI_API_KEY="your_api_key"
export SYS_SENTIENT_SERVER_API_KEY="your_dashboard_key" # recommended
./sys-daemon
```

Access the dashboard at `http://localhost:8080`.
If API authentication is enabled, build the web UI with
`VITE_SYS_SENTIENT_API_KEY` set to the same dashboard key.

By default, the daemon keeps 24 hours of metrics and 7 days of AI insight
history to keep the local SQLite database bounded.

## Configuration

Configuration can be set via `config.yaml` or Environment Variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `SYS_SENTIENT_SERVER_PORT` | 8080 | Web Server Port |
| `SYS_SENTIENT_SERVER_API_KEY` | - | Optional API key for `/api/*` and `/ws/*` |
| `SYS_SENTIENT_SERVER_ALLOWED_ORIGINS` | `http://localhost:8080,http://localhost:5173` | CORS/WebSocket origin allowlist |
| `SYS_SENTIENT_GEMINI_API_KEY` | - | Gemini API key; AI is disabled when omitted |
| `SYS_SENTIENT_GEMINI_MODEL_NAME` | gemini-2.5-flash-lite | AI Model |
| `SYS_SENTIENT_COLLECTOR_POLL_INTERVAL_SECONDS` | 2 | Metrics Poll Rate |

## Installation (Linux Systemd)

1. Create a dedicated service account.
   ```bash
   sudo useradd --system --home-dir /var/lib/sys-sentient --shell /usr/sbin/nologin --groups systemd-journal sys-sentient
   ```
2. Move binary and web assets to `/opt/sys-sentient`.
   ```bash
   sudo mkdir -p /opt/sys-sentient
   sudo cp sys-daemon /opt/sys-sentient/
   sudo cp -r web/dist /opt/sys-sentient/web/dist
   sudo chown -R root:root /opt/sys-sentient
   ```
3. Install service file.
   ```bash
   sudo cp sys-sentient.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now sys-sentient
   ```

## Features

- Real-time CPU, memory, swap, disk, network, load, and temperature metrics.
- Top processes list with CPU, memory, and user context.
- **AI Analysis**: Detects anomalies and provides health summaries using Gemini AI.
- PII Scrubbing: Automatically redacts sensitive info (IPs, Emails) before sending to AI.
