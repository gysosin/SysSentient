# SysSentient Development Plan

## Phase 1: The Skeleton (Core Daemon & Storage)
Goal: Establish the base Go project, collect system metrics, and store them locally.

- [x] **Project Setup**
    - [x] Initialize Go module (`go mod init sys-sentient`)
    - [x] Create directory structure (`cmd/daemon`, `cmd/cli`, `internal/collector`, `internal/storage`, `internal/config`)
    - [x] Clean up existing unrelated files in the repository (if any)
- [x] **Metric Collection (The Collector)**
    - [x] Install `gopsutil` dependency
    - [x] Implement `GetSystemMetrics()` function (CPU, RAM, usage calculations)
    - [x] Implement `GetDiskMetrics()` (I/O, Space)
    - [x] Implement `GetNetworkMetrics()` (Bandwidth)
    - [x] Create a Polling Loop (Ticker) running every 2 seconds
- [x] **Data Persistence (The Memory)**
    - [x] Install `go-sqlite3` driver
    - [x] Design SQLite Schema (`metrics` table: timestamp, cpu_percent, ram_usage, disk_io, etc.)
    - [x] Implement `MetricStore` interface for saving/retrieving history
    - [x] Verify database creation and write operations

## Phase 2: The Brain (Gemini AI Integration)
Goal: Connect to Google Gemini API securely and implement the analysis logic.

- [ ] **Configuration & Security**
    - [ ] Define Config Structure (YAML)
    - [ ] Implement Config Loader (reads from `/etc/sys-sentient/config.yaml` or local dev path)
    - [ ] Ensure API Key is never hardcoded (load from config/env)
- [ ] **PII Scrubbing (Privacy Layer)**
    - [ ] Implement Regex patterns for IP Addresses (IPv4/v6)
    - [ ] Implement Regex for Email Addresses
    - [ ] Implement Regex for Usernames in file paths
    - [ ] Create `SanitizeLog(string) string` function and unit test it
- [ ] **Gemini Client Implementation**
    - [ ] Integrate `google.golang.org/genai` SDK
    - [ ] Implement `AnalyzeSystemState(metrics, logs)` function
    - [ ] Create Prompt Engineering logic (convert JSON metrics -> English prompt)
    - [ ] Implement Cost/Rate controls (limit to daily cap)
- [ ] **Analysis Triggers**
    - [ ] Implement Threshold Monitor (CPU > 80% for 2m, RAM < 500MB)
    - [ ] Integrate Process Metadata collection (Top 5 processes) for context
    - [ ] Integrate Log Reading (`dmesg`, `journalctl` tailing) - safe read-only access

## Phase 3: The Interface (Web Client)
Goal: Use the provided React/Vite template as the user interface.

- [ ] **Daemon API Server**
    - [ ] Add `net/http` server to Daemon
    - [ ] Create `/api/metrics` endpoint (JSON response)
    - [ ] Enable CORS for local development
- [ ] **Web Client Setup**
    - [ ] Move into `web/` directory
    - [ ] Install Node.js dependencies (`npm install`)
    - [ ] Configure API endpoint in `constants.ts` or `.env`
- [ ] **Dashboard Implementation**
    - [ ] Connect React App to Daemon API
    - [ ] Visualize Real-time Metrics (Recharts or Sparklines)
    - [ ] Display "AI Insight" section
- [ ] **"Ask AI" Feature**
    - [ ] Add "Analyze" button in UI
    - [ ] Call `/api/analyze` endpoint on Daemon

## Phase 4: System Integration & Polish
Goal: Make it a proper system service.

- [ ] **Service Management**
    - [ ] Create `sys-sentient.service` systemd unit file
    - [ ] Ensure Daemon serves the Web Client statics (optional) or run separately
- [ ] **End-to-End Testing**
    - [ ] Verify "Real Data" flow (Daemon -> API -> Web UI)
    - [ ] Test Fallback mechanisms (No Internet behavior)
- [ ] **Documentation**
    - [ ] Update README.md with installation and usage instructions
