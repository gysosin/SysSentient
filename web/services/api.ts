import { SystemMetrics, AIAnalysisResult, Process } from '../types';
import { API_BASE_URL } from '../constants';

export const fetchMetricsHistory = async (): Promise<{metrics: SystemMetrics[], processes: Process[]}> => {
    try {
        const response = await fetch(`${API_BASE_URL}/metrics`);
        if (!response.ok) throw new Error('Failed to fetch metrics');
        const rawData = await response.json(); // Array of models.SystemState
        
        // rawData is ordered DESC (newest first).
        const sorted = rawData.reverse(); // Now Oldest -> Newest

        const metrics: SystemMetrics[] = [];
        let latestProcesses: Process[] = [];

        for (let i = 0; i < sorted.length; i++) {
            const curr = sorted[i];
            const prev = i > 0 ? sorted[i-1] : null;
            
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
                memoryUsed: (curr.memory_used || 0) / 1024 / 1024,
                memoryTotal: (curr.memory_total || 1) / 1024 / 1024,
                temperature: 0, 
                diskRead: Math.max(0, diskReadRate / 1024 / 1024), // MB/s
                diskWrite: Math.max(0, diskWriteRate / 1024 / 1024),
                networkRx: Math.max(0, netRxRate / 1024), // KB/s
                networkTx: Math.max(0, netTxRate / 1024)
            });

            // Parse processes from the latest entry
            if (i === sorted.length - 1 && curr.top_processes) {
                latestProcesses = parseProcesses(curr.top_processes);
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
        const response = await fetch(`${API_BASE_URL}/analyze`, { method: 'POST' });
        if (!response.ok) throw new Error('Failed to analyze');
        const data = await response.json();
        
        return {
            status: 'Warning', // Default
            summary: "AI Analysis Generated",
            detailedAnalysis: data.insight || "No details provided",
            recommendedActions: []
        };
    } catch (e) {
        console.error("API Error triggerAnalysis", e);
        return null;
    }
}

export const fetchLatestInsight = async (): Promise<AIAnalysisResult | null> => {
     try {
        const response = await fetch(`${API_BASE_URL}/insights`);
        if (!response.ok) throw new Error('Failed to fetch insights');
        const data = await response.json(); // Array of {timestamp, content}
        
        if (data && data.length > 0) {
            return {
                status: 'Healthy', // Placeholder
                summary: "Recent Insight",
                detailedAnalysis: data[0].content,
                recommendedActions: []
            };
        }
        return null;
    } catch (e) {
        console.error("API Error fetchLatestInsight", e);
        return null;
    }
}

function parseProcesses(procStr: string): Process[] {
    // Format: "name (cpu%), name (cpu%)"
    // Example: "chrome (12.5%), code (5.0%)"
    if (!procStr || procStr === "None") return [];
    
    return procStr.split(', ').map((p, idx) => {
        const parts = p.match(/(.+) \(([\d.]+)%\)/);
        if (parts) {
            return {
                pid: 1000 + idx, // Mock PID
                name: parts[1],
                user: 'root', // Mock user
                cpu: parseFloat(parts[2]),
                memory: 0, // Mock memory
                state: 'Running'
            };
        }
        return {
             pid: 0, name: p, user: '?', cpu: 0, memory: 0, state: 'Running'
        };
    });
}
