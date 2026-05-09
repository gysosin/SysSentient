import { SystemMetrics, AIAnalysisResult, Process } from '../types';
import { API_BASE_URL, authHeaders } from '../constants';

export const fetchMetricsHistory = async (): Promise<{ metrics: SystemMetrics[], processes: Process[] }> => {
    try {
        const response = await fetch(`${API_BASE_URL}/metrics`, {
            headers: authHeaders(),
        });
        if (!response.ok) throw new Error('Failed to fetch metrics');
        const rawData = await response.json(); // Array of models.SystemState

        // rawData is ordered DESC (newest first).
        const sorted = rawData.reverse(); // Now Oldest -> Newest

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
                    diskReadRate = (curr.disk_read_bytes - prev.disk_read_bytes) / dt;
                    diskWriteRate = (curr.disk_write_bytes - prev.disk_write_bytes) / dt;
                    netRxRate = (curr.net_recv_bytes - prev.net_recv_bytes) / dt;
                    netTxRate = (curr.net_sent_bytes - prev.net_sent_bytes) / dt;
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

export const triggerAnalysis = async (): Promise<AIAnalysisResult | null> => {
    try {
        const response = await fetch(`${API_BASE_URL}/analyze`, {
            method: 'POST',
            headers: authHeaders(),
        });
        if (!response.ok) throw new Error('Failed to analyze');
        const data = await response.json();

        // Data should be the AIAnalysis object
        return {
            status: data.status || 'Warning',
            summary: data.summary || "AI Analysis Generated",
            detailedAnalysis: data.detailedAnalysis || "No details provided",
            recommendedActions: data.recommendedActions || []
        };
    } catch (e) {
        console.error("API Error triggerAnalysis", e);
        return null;
    }
}

export const fetchLatestInsight = async (): Promise<AIAnalysisResult | null> => {
    try {
        const response = await fetch(`${API_BASE_URL}/insights`, {
            headers: authHeaders(),
        });
        if (!response.ok) throw new Error('Failed to fetch insights');
        const data = await response.json(); // Array of {timestamp, content}

        if (data && data.length > 0) {
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

function normalizeProcess(process: any): Process {
    return {
        pid: Number(process.pid) || 0,
        name: String(process.name || '?'),
        user: String(process.user || '?'),
        cpu: Number(process.cpu) || 0,
        memory: Number(process.memory) || 0,
        state: normalizeProcessState(process.state)
    };
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
