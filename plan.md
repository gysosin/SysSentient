# Project Plan

> **Accuracy note.** Several items below were previously marked `[x]` but were
> not implemented. They have been corrected in place with an explanation rather
> than quietly reworded, so the history stays honest. The active roadmap is the
> phased plan tracked separately; this file is the original single-host plan.

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

- [x] **Configuration & Security**
    - [x] Define Config Structure (YAML)
    - [x] Implement Config Loader (reads from `/etc/sys-sentient/config.yaml` or local dev path)
    - [x] Ensure API Key is never hardcoded (load from config/env)
- [x] **PII Scrubbing (Privacy Layer)**
    - [x] Implement Regex patterns for IP Addresses (IPv4/v6)
    - [x] Implement Regex for Email Addresses
    - [x] Implement Regex for Usernames in file paths
    - [x] Create `SanitizeLog(string) string` function and unit test it
- [x] **Gemini Client Implementation**
    - [x] Integrate `google.golang.org/genai` SDK
    - [x] Implement `AnalyzeSystemState(metrics, logs)` function
    - [x] Create Prompt Engineering logic (convert JSON metrics -> English prompt)
    - [x] Implement Cost/Rate controls (limit to daily cap) - *Daily USD cap enforced per UTC day in `internal/ai/budget.go`; the 5-minute cooldown remains as a second limiter.*
- [x] **Analysis Triggers**
    - [x] Implement Threshold Monitor (CPU > 80% for 2m, RAM < 500MB) - *Durations are now real: `internal/alerting` requires a condition to hold for the rule's `For` window before firing. The original implementation was single-sample despite this line claiming "for 2m".*
    - [x] Integrate Process Metadata collection (Top 5 processes) for context
    - [x] Integrate Log Reading (`dmesg`, `journalctl` tailing) - *Placeholder implemented*

## Phase 3: The Interface (Web Client)
Goal: Use the provided React/Vite template as the user interface.

- [x] **Daemon API Server**
    - [x] Add `net/http` server to Daemon
    - [x] Create `/api/metrics` endpoint (JSON response)
    - [x] Enable CORS for local development
- [x] **Web Client Setup**
    - [x] Move into `web/` directory
    - [x] Install Node.js dependencies (`npm install`)
    - [x] Configure API endpoint in `constants.ts` or `.env`
- [x] **Dashboard Implementation**
    - [x] Connect React App to Daemon API
    - [x] Visualize Real-time Metrics (Recharts or Sparklines)
    - [x] Display "AI Insight" section
- [x] **"Ask AI" Feature**
    - [x] Add "Analyze" button in UI
    - [x] Call `/api/analyze` endpoint on Daemon

## Phase 4: System Integration & Polish
Goal: Make it a proper system service.

- [x] **Service Management**
    - [x] Create `sys-sentient.service` systemd unit file
    - [x] Ensure Daemon serves the Web Client statics (optional) or run separately
- [x] **End-to-End Testing**
    - [x] Verify "Real Data" flow (Daemon -> API -> Web UI)
    - [x] Test Fallback mechanisms (No Internet behavior)
- [x] **Documentation**
    - [x] Update README.md with installation and usage instructions

## Phase 5: Scalability & Intelligence Optimization

Goal: Reduce Gemini API costs, improve context with RAG, and optimize local performance for heavy loads.



- [x] **Database Optimization**

    - [x] Enable SQLite WAL (Write-Ahead Logging) mode for concurrent read/write.

    - [x] Implement Data Retention Policy (Auto-prune metrics > 24h, or downsample).

    - [x] Add Database Indexing on timestamp columns for faster retrieval.

- [x] **Smart AI Context (RAG)**

    - [ ] Integrate a lightweight Vector Store (e.g., `chromem-go` or simple vector cache). - *NOT DONE. `internal/ai/rag.go` is a SHA-256 hash of the log text, not a vector store: no embeddings, no similarity search. One byte of drift is a cache miss, and the cache is in-memory only so it is lost on restart.*

    - [x] Implement Log Deduplication (Collapse repeated logs before analysis).

    - [ ] Implement "Insight Caching": Store embedding of problem -> Insight. If similar problem occurs, return cached insight instead of calling API. - *NOT DONE. Exact-string matching only; "similar" is not implemented. See `architecture_review.md` lines 36-39, whose "top 3 related past incidents" context injection was never built, so the AI still has no memory of yesterday's incident.*

- [ ] **Performance Tuning**

    - [ ] Optimize Metric Collector (Avoid memory allocations in hot loops).

    - [ ] Implement Adaptive Polling (Slow down polling when system is idle, speed up under load).
