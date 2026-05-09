import { SystemMetrics, AIAnalysisResult, Process, LogEntry } from '../types';
import { API_BASE_URL, authHeaders } from '../constants';

interface LogsResponse {
    collectedAt?: string;
    content?: string;
}

interface ErrorResponse {
    error?: string;
}

interface RawProcess {
    pid?: number;
    name?: string;
    user?: string;
    cpu?: number;
    memory?: number;
    state?: unknown;
}

interface RawSystemState {
    timestamp: string;
    cpu_usage?: number;
    cpu_per_core?: number[];
    memory_used?: number;
    memory_total?: number;
    swap_used?: number;
    swap_total?: number;
    temperature?: number;
    disk_read_bytes?: number;
    disk_write_bytes?: number;
    disk_iops?: number;
    net_recv_bytes?: number;
    net_sent_bytes?: number;
    load_avg_1?: number;
    load_avg_5?: number;
    load_avg_15?: number;
    processes?: RawProcess[];
    top_processes?: string;
}

interface InsightRecord {
    content?: string;
}

export const fetchMetricsHistory = async (): Promise<{ metrics: SystemMetrics[], processes: Process[] }> => {
    try {
        const response = await fetch(`${API_BASE_URL}/metrics`, {
            headers: authHeaders(),
        });
        if (!response.ok) throw new Error('Failed to fetch metrics');
        const rawData = await response.json() as RawSystemState[]; // Array of models.SystemState
        if (!Array.isArray(rawData)) {
            return { metrics: [], processes: [] };
        }

        // rawData is ordered DESC (newest first).
        const sorted = [...rawData].reverse(); // Now Oldest -> Newest

        const metrics: SystemMetrics[] = [];
        let latestProcesses: Process[] = [];

        for (let i = 0; i < sorted.length; i++) {
            const curr = sorted[i];
            const prev = i > 0 ? sorted[i - 1] : null;

            let diskReadRate = 0;
            let diskWriteRate = 0;
            let netRxRate = 0;
            let netTxRate = 0;

            const t1 = new Date(curr.timestamp).getTime();

            if (prev) {
                const t0 = new Date(prev.timestamp).getTime();
                const dt = (t1 - t0) / 1000;

                if (dt > 0) {
                    diskReadRate = ((curr.disk_read_bytes ?? 0) - (prev.disk_read_bytes ?? 0)) / dt;
                    diskWriteRate = ((curr.disk_write_bytes ?? 0) - (prev.disk_write_bytes ?? 0)) / dt;
                    netRxRate = ((curr.net_recv_bytes ?? 0) - (prev.net_recv_bytes ?? 0)) / dt;
                    netTxRate = ((curr.net_sent_bytes ?? 0) - (prev.net_sent_bytes ?? 0)) / dt;
                }
            }

            metrics.push({
                timestamp: t1,
                cpuLoad: curr.cpu_usage || 0,
                cpuPerCore: curr.cpu_per_core || [],
                memoryUsed: (curr.memory_used || 0) / 1024 / 1024,
                memoryTotal: (curr.memory_total || 1) / 1024 / 1024,
                swapUsed: (curr.swap_used || 0) / 1024 / 1024,
                swapTotal: (curr.swap_total || 0) / 1024 / 1024,
                temperature: curr.temperature || 0,
                diskRead: Math.max(0, diskReadRate / 1024 / 1024), // MB/s
                diskWrite: Math.max(0, diskWriteRate / 1024 / 1024),
                diskIOPS: curr.disk_iops || 0,
                networkRx: Math.max(0, netRxRate / 1024), // KB/s
                networkTx: Math.max(0, netTxRate / 1024),
                loadAvg1: curr.load_avg_1 || 0,
                loadAvg5: curr.load_avg_5 || 0,
                loadAvg15: curr.load_avg_15 || 0,
            });

            // Parse processes from the latest entry.
            if (i === sorted.length - 1) {
                if (Array.isArray(curr.processes) && curr.processes.length > 0) {
                    latestProcesses = curr.processes.map(normalizeProcess);
                } else if (curr.top_processes) {
                    latestProcesses = parseProcesses(curr.top_processes);
                }
            }
        }

        return { metrics, processes: latestProcesses };

    } catch (e) {
        console.error("API Error fetchMetricsHistory", e);
        return { metrics: [], processes: [] };
    }
}

export const triggerAnalysis = async (): Promise<AIAnalysisResult> => {
    const response = await fetch(`${API_BASE_URL}/analyze`, {
        method: 'POST',
        headers: authHeaders(),
    });
    if (!response.ok) {
        throw new Error(await readAPIError(response, 'Failed to analyze'));
    }

    const data = await response.json() as Partial<AIAnalysisResult>;

    // Data should be the AIAnalysis object
    return {
        status: data.status || 'Warning',
        summary: data.summary || "AI Analysis Generated",
        detailedAnalysis: data.detailedAnalysis || "No details provided",
        recommendedActions: data.recommendedActions || []
    };
}

export const fetchLatestInsight = async (): Promise<AIAnalysisResult | null> => {
    try {
        const response = await fetch(`${API_BASE_URL}/insights`, {
            headers: authHeaders(),
        });
        if (!response.ok) throw new Error('Failed to fetch insights');
        const data = await response.json() as InsightRecord[]; // Array of {timestamp, content}

        if (Array.isArray(data) && data.length > 0 && data[0].content) {
            // Content is a JSON string now
            try {
                const parsed = JSON.parse(data[0].content);
                return {
                    status: parsed.status || 'Healthy',
                    summary: parsed.summary || "Recent Insight",
                    detailedAnalysis: parsed.detailedAnalysis,
                    recommendedActions: parsed.recommendedActions || []
                };
            } catch (parseErr) {
                // Fallback for old insights that were plain text
                return {
                    status: 'Healthy',
                    summary: "Legacy Insight",
                    detailedAnalysis: data[0].content,
                    recommendedActions: []
                };
            }
        }
        return null;
    } catch (e) {
        console.error("API Error fetchLatestInsight", e);
        return null;
    }
}

export const fetchRecentLogs = async (): Promise<LogEntry[]> => {
    try {
        const response = await fetch(`${API_BASE_URL}/logs`, {
            headers: authHeaders(),
        });
        if (!response.ok) throw new Error('Failed to fetch logs');

        const data = await response.json() as LogsResponse;
        return parseLogContent(data.content || '', data.collectedAt || new Date().toISOString());
    } catch (e) {
        console.error("API Error fetchRecentLogs", e);
        return [];
    }
}

function normalizeProcess(process: RawProcess): Process {
    return {
        pid: Number(process.pid) || 0,
        name: String(process.name || '?'),
        user: String(process.user || '?'),
        cpu: Number(process.cpu) || 0,
        memory: Number(process.memory) || 0,
        state: normalizeProcessState(process.state)
    };
}

async function readAPIError(response: Response, fallback: string): Promise<string> {
    try {
        const payload = await response.json() as ErrorResponse;
        return payload.error || fallback;
    } catch {
        return fallback;
    }
}

function normalizeProcessState(state: unknown): Process['state'] {
    if (state === 'Running' || state === 'Sleeping' || state === 'Zombie' || state === 'Stopped') {
        return state;
    }
    return 'Running';
}

function parseProcesses(procStr: string): Process[] {
    // Format: "name (cpu%, memoryMB, user), name (cpu%, memoryMB, user)"
    // Example: "chrome (12.5%, 512MB, alice), code (5.0%, 256MB, alice)"
    if (!procStr || procStr === "None") return [];

    const processes: Process[] = [];
    const processPattern = /(.+?) \(([\d.]+)%, (\d+)MB, ([^)]+)\)(?:, |$)/g;
    let match: RegExpExecArray | null;

    while ((match = processPattern.exec(procStr)) !== null) {
        processes.push({
            pid: 1000 + processes.length, // Backend currently exposes a summary string, not PID.
            name: match[1],
            user: match[4],
            cpu: parseFloat(match[2]),
            memory: parseInt(match[3], 10),
            state: 'Running'
        });
    }

    if (processes.length > 0) return processes;

    return procStr.split(', ').map((p) => {
        return {
            pid: 0, name: p, user: '?', cpu: 0, memory: 0, state: 'Running'
        };
    });
}

function parseLogContent(content: string, collectedAt: string): LogEntry[] {
    return content
        .split('\n')
        .map(line => line.trim())
        .filter(Boolean)
        .slice(-80)
        .map((line) => ({
            timestamp: collectedAt,
            facility: detectFacility(line),
            level: detectLevel(line),
            message: line,
        }));
}

function detectFacility(line: string): string {
    const lower = line.toLowerCase();
    if (lower.includes('journal')) return 'systemd';
    if (lower.includes('kernel') || lower.includes('dmesg')) return 'kernel';
    if (lower.includes('auth') || lower.includes('sudo') || lower.includes('ssh')) return 'auth';
    if (line.startsWith('===')) return 'source';
    return 'system';
}

function detectLevel(line: string): LogEntry['level'] {
    const lower = line.toLowerCase();
    if (lower.includes('error') || lower.includes('fail') || lower.includes('critical') || lower.includes('panic')) {
        return 'ERROR';
    }
    if (lower.includes('warn')) {
        return 'WARN';
    }
    return 'INFO';
}
