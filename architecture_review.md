# Architecture Review & Optimization Strategy

## Executive Summary
The current `SysSentient` architecture (Go Daemon + SQLite + Direct Gemini API) is a solid foundation for a single-node monitor. However, for "heavy system load" and cost-efficiency at scale, the current "send-everything-to-AI" approach is risky and expensive.

We **do not** need heavy infrastructure (Postgres, Redis, NATS) for a single-node instance. Adding them would increase complexity and resource usage (CPU/RAM) without proportional benefit. Instead, we should focus on **Application-Layer Intelligence** (Vector Search, RAG) and **Data Efficiency** (smart sampling).

## 1. Infrastructure Analysis

| Technology | Status | Verdict for SysSentient (Single Node) |
| :--- | :--- | :--- |
| **PostgreSQL** | ❌ Not Needed | SQLite is faster for local reads/writes and has zero deployment overhead. Only switch if monitoring >10 nodes centrally. |
| **Redis** | ❌ Not Needed | Go's in-memory `sync.Map` or LRU cache is nanosecond-fast. Redis adds serialization overhead and maintenance cost. |
| **NATS** | ❌ Not Needed | Essential for distributed systems (Microservices). For a daemon talking to itself/API, Go Channels are the "internal NATS". |
| **Object Storage (S3)** | ❌ Not Needed | Logs and metrics are text-heavy and compress well. Local disk or SQLite is sufficient for retention < 30 days. |
| **Vector Storage** | ✅ **Recommended** | **Critical for AI Cost Reduction.** Instead of sending logs to Gemini blindly, we embed them to find *patterns*. |

## 2. Identified Bottlenecks & Risks

### A. The "Token Drain" (Cost & Latency)
**Current:** Every time analysis is triggered, we send raw logs/metrics to Gemini.
**Risk:**
- **Cost:** High token usage for repetitive logs.
- **Context:** The AI has no memory of yesterday's crash, so it might give generic advice repeatedly.

### B. High Load Performance
**Current:** `metrics` table grows indefinitely.
**Risk:**
- Queries get slower over time.
- Heavy IO if the system is already under load.

## 3. Optimization Strategy (The "Phase 5" Plan)

### A. Intelligent "RAG" (Retrieval Augmented Generation)
Instead of asking "What is wrong?" with *all* data:
1.  **Vector Embeddings:** Use a lightweight local Go embedder (or small API call) to turn log entries into vectors.
2.  **Pattern Matching:** Store these in a local Vector Store (e.g., `sqlite-vec` or a simple in-memory vector cache).
3.  **Deduplication:** If the current error vector matches a "known issue" vector from yesterday, **don't call Gemini**. Show the cached insight.
4.  **Context Injection:** If it *is* new, send it to Gemini, but include the "Top 3 related past incidents" from the vector store.

### B. SQLite Optimization
1.  **WAL Mode:** Enable Write-Ahead Logging for better concurrency.
2.  **Retention Policy:** Implement a "Janitor" routine to delete metrics older than 24h or downsample them (avg of 5 mins).

### C. Smart Sampling
- Don't analyze *every* spike. Implement a "Time-to-Trigger" (e.g., CPU > 90% for > 30s).
- **Log Debouncing:** If a log line repeats 1000 times, collapse it to `[x1000] Error: ...` before sending to AI.

## 4. Technology Selection for Phase 5
- **Vector DB:** `chromem-go` (In-memory) or simple cosine similarity on local JSON for simplicity. No heavy external DB.
- **Embeddings:** Small local model (ONNX via Go) or reduced-dimension API embeddings.

## Conclusion
We will stay "lightweight" by optimizing *logic* rather than adding *infrastructure*. The goal is **Local Intelligence** to minimize **Remote Intelligence (API)** costs.
