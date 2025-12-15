# SysSentient

SysSentient is an intelligent system monitor that uses Google Gemini AI to analyze system metrics and logs.

## Architecture

- **Daemon (`sys-daemon`)**: Go application that collects metrics, stores them in SQLite, and exposes a JSON API. It also handles AI analysis.
- **Web UI**: React application that visualizes metrics and provides an interface for AI insights.

## Prerequisites

- Go 1.25+
- Node.js 18+
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

## Running

Ensure `web/dist` exists (from step 1).

```bash
export SYS_SENTIENT_GEMINI_API_KEY="your_api_key"
./sys-daemon
```

Access the dashboard at `http://localhost:8080`.

## Configuration

Configuration can be set via `config.yaml` or Environment Variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `SYS_SENTIENT_SERVER_PORT` | 8080 | Web Server Port |
| `SYS_SENTIENT_GEMINI_API_KEY` | - | **Required** Gemini API Key |
| `SYS_SENTIENT_GEMINI_MODEL_NAME` | gemini-1.5-flash | AI Model |
| `SYS_SENTIENT_COLLECTOR_POLL_INTERVAL_SECONDS` | 2 | Metrics Poll Rate |

## Installation (Linux Systemd)

1. Move binary and web assets to `/opt/sys-sentient`.
   ```bash
   sudo mkdir -p /opt/sys-sentient
   sudo cp sys-daemon /opt/sys-sentient/
   sudo cp -r web/dist /opt/sys-sentient/web/dist
   ```
2. Install service file.
   ```bash
   sudo cp sys-sentient.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now sys-sentient
   ```

## Features

- Real-time CPU, Memory, Disk, Network metrics.
- Top processes list.
- **AI Analysis**: Detects anomalies and provides health summaries using Gemini AI.
- PII Scrubbing: Automatically redacts sensitive info (IPs, Emails) before sending to AI.
