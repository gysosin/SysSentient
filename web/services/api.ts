import type { SystemMetrics, AIAction, AIAnalysisResult, Process, LogEntry } from '../types';
import { API_BASE_URL, authHeaders } from '../constants.js';

const API_REQUEST_TIMEOUT_MS = 8000;

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

const AI_STATUSES = new Set<AIAnalysisResult['status']>(['Healthy', 'Warning', 'Critical']);

export const fetchMetricsHistory = async (): Promise<{ metrics: SystemMetrics[], processes: Process[] }> => {
    try {
        const response = await fetchWithTimeout(`${API_BASE_URL}/metrics`, {
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
        let previous: { state: RawSystemState; timestamp: number } | null = null;

        for (const curr of sorted) {
            let diskReadRate = 0;
            let diskWriteRate = 0;
            let netRxRate = 0;
            let netTxRate = 0;

            const t1 = Date.parse(curr.timestamp);
            if (!Number.isFinite(t1)) {
                continue;
            }

            if (previous) {
                const dt = (t1 - previous.timestamp) / 1000;

                if (dt > 0) {
                    diskReadRate = (finiteNumber(curr.disk_read_bytes) - finiteNumber(previous.state.disk_read_bytes)) / dt;
                    diskWriteRate = (finiteNumber(curr.disk_write_bytes) - finiteNumber(previous.state.disk_write_bytes)) / dt;
                    netRxRate = (finiteNumber(curr.net_recv_bytes) - finiteNumber(previous.state.net_recv_bytes)) / dt;
                    netTxRate = (finiteNumber(curr.net_sent_bytes) - finiteNumber(previous.state.net_sent_bytes)) / dt;
                }
            }

            metrics.push({
                timestamp: t1,
                cpuLoad: finiteNumber(curr.cpu_usage),
                cpuPerCore: finiteNumberArray(curr.cpu_per_core),
                memoryUsed: finiteNumber(curr.memory_used) / 1024 / 1024,
                memoryTotal: positiveFiniteNumber(curr.memory_total, 1) / 1024 / 1024,
                swapUsed: finiteNumber(curr.swap_used) / 1024 / 1024,
                swapTotal: finiteNumber(curr.swap_total) / 1024 / 1024,
                temperature: finiteNumber(curr.temperature),
                diskRead: Math.max(0, diskReadRate / 1024 / 1024), // MB/s
                diskWrite: Math.max(0, diskWriteRate / 1024 / 1024),
                diskIOPS: finiteNumber(curr.disk_iops),
                networkRx: Math.max(0, netRxRate / 1024), // KB/s
                networkTx: Math.max(0, netTxRate / 1024),
                loadAvg1: finiteNumber(curr.load_avg_1),
                loadAvg5: finiteNumber(curr.load_avg_5),
                loadAvg15: finiteNumber(curr.load_avg_15),
            });

            if (Array.isArray(curr.processes) && curr.processes.length > 0) {
                latestProcesses = curr.processes.map(normalizeProcess);
            } else if (curr.top_processes) {
                latestProcesses = parseProcesses(curr.top_processes);
            } else {
                latestProcesses = [];
            }
            previous = { state: curr, timestamp: t1 };
        }

        return { metrics, processes: latestProcesses };

    } catch (e) {
        console.error("API Error fetchMetricsHistory", e);
        return { metrics: [], processes: [] };
    }
}

export const triggerAnalysis = async (): Promise<AIAnalysisResult> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/analyze`, {
        method: 'POST',
        headers: authHeaders(),
    });
    if (!response.ok) {
        throw new Error(await readAPIError(response, 'Failed to analyze'));
    }

    const data = await response.json() as unknown;

    return normalizeAnalysisResult(data, {
        fallbackStatus: 'Warning',
        fallbackSummary: 'AI Analysis Generated',
        fallbackDetails: 'No details provided',
    });
}

export const fetchLatestInsight = async (): Promise<AIAnalysisResult | null> => {
    try {
        const response = await fetchWithTimeout(`${API_BASE_URL}/insights`, {
            headers: authHeaders(),
        });
        if (!response.ok) throw new Error('Failed to fetch insights');
        const data = await response.json() as InsightRecord[]; // Array of {timestamp, content}

        if (Array.isArray(data) && data.length > 0 && data[0].content) {
            // Content is a JSON string now
            try {
                const parsed = JSON.parse(data[0].content) as unknown;
                return normalizeAnalysisResult(parsed, {
                    fallbackStatus: 'Warning',
                    fallbackSummary: 'Recent Insight',
                    fallbackDetails: 'No details provided',
                });
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
        const response = await fetchWithTimeout(`${API_BASE_URL}/logs`, {
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
        pid: Math.max(0, Math.trunc(finiteNumber(process.pid))),
        name: nonEmptyString(process.name, '?'),
        user: nonEmptyString(process.user, '?'),
        cpu: finiteNumber(process.cpu),
        memory: finiteNumber(process.memory),
        state: normalizeProcessState(process.state)
    };
}

function normalizeAnalysisResult(
    value: unknown,
    options: {
        fallbackStatus: AIAnalysisResult['status'];
        fallbackSummary: string;
        fallbackDetails: string;
    }
): AIAnalysisResult {
    const record = asRecord(value);
    const status = record?.status;

    return {
        status: typeof status === 'string' && AI_STATUSES.has(status as AIAnalysisResult['status'])
            ? status as AIAnalysisResult['status']
            : options.fallbackStatus,
        summary: nonEmptyString(record?.summary, options.fallbackSummary),
        detailedAnalysis: nonEmptyString(record?.detailedAnalysis, options.fallbackDetails),
        recommendedActions: normalizeRecommendedActions(record?.recommendedActions),
    };
}

function normalizeRecommendedActions(value: unknown): AIAction[] {
    if (!Array.isArray(value)) {
        return [];
    }

    return value.flatMap((item, index) => {
        const action = asRecord(item);
        if (!action) {
            return [];
        }

        const command = nonEmptyString(action.command, '');
        if (!command) {
            return [];
        }

        return [{
            id: nonEmptyString(action.id, `action-${index + 1}`),
            command,
            description: nonEmptyString(action.description, 'No description provided'),
            isSafe: action.isSafe === true,
        }];
    });
}

function asRecord(value: unknown): Record<string, unknown> | null {
    return value && typeof value === 'object' && !Array.isArray(value)
        ? value as Record<string, unknown>
        : null;
}

function nonEmptyString(value: unknown, fallback: string): string {
    return typeof value === 'string' && value.trim() ? value : fallback;
}

function finiteNumber(value: unknown, fallback = 0): number {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
}

function positiveFiniteNumber(value: unknown, fallback: number): number {
    const parsed = finiteNumber(value, fallback);
    return parsed > 0 ? parsed : fallback;
}

function finiteNumberArray(value: unknown): number[] {
    if (!Array.isArray(value)) {
        return [];
    }
    return value.map((item) => finiteNumber(item));
}

async function fetchWithTimeout(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const controller = new AbortController();
    const timeoutID = setTimeout(() => controller.abort(), API_REQUEST_TIMEOUT_MS);

    try {
        return await fetch(input, {
            ...init,
            signal: controller.signal,
        });
    } catch (error) {
        if (error instanceof DOMException && error.name === 'AbortError') {
            throw new Error('Request timed out');
        }
        throw error;
    } finally {
        clearTimeout(timeoutID);
    }
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
    // Format: "name (cpu%, memoryMB)" with legacy support for ", user".
    // Example: "chrome (12.5%, 512MB), code (5.0%, 256MB)"
    if (!procStr || procStr === "None") return [];

    const processes: Process[] = [];
    const processPattern = /(.+?) \(([\d.]+)%, (\d+)MB(?:, ([^)]+))?\)(?:, |$)/g;
    let match: RegExpExecArray | null;

    while ((match = processPattern.exec(procStr)) !== null) {
        processes.push({
            pid: 1000 + processes.length, // Backend currently exposes a summary string, not PID.
            name: match[1],
            user: match[4] || '?',
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
