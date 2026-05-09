import { useState, useEffect, useCallback, useRef } from 'react';
import { SystemMetrics } from '../types';
import { metricsWebSocketURL } from '../constants';

interface RawMetricsPayload {
    timestamp: string;
    cpu_usage?: number;
    cpu_per_core?: number[];
    memory_used?: number;
    memory_total?: number;
    swap_used?: number;
    swap_total?: number;
    disk_read_bytes?: number;
    disk_write_bytes?: number;
    disk_iops?: number;
    net_recv_bytes?: number;
    net_sent_bytes?: number;
    load_avg_1?: number;
    load_avg_5?: number;
    load_avg_15?: number;
    temperature?: number;
}

type WSMessage = {
    type: 'metrics';
    payload: RawMetricsPayload;
} | {
    type: 'alert' | 'prediction';
    payload: unknown;
}

interface UseWebSocketReturn {
    connected: boolean;
    latestMetrics: SystemMetrics | null;
    metricsHistory: SystemMetrics[];
    reconnect: () => void;
}

const MAX_HISTORY = 60; // Keep last 60 data points

interface RawCounters {
    timestamp: number;
    diskReadBytes: number;
    diskWriteBytes: number;
    netRecvBytes: number;
    netSentBytes: number;
}

export function useWebSocket(): UseWebSocketReturn {
    const [connected, setConnected] = useState(false);
    const [latestMetrics, setLatestMetrics] = useState<SystemMetrics | null>(null);
    const [metricsHistory, setMetricsHistory] = useState<SystemMetrics[]>([]);
    const wsRef = useRef<WebSocket | null>(null);
    const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
    const reconnectAttemptsRef = useRef(0);
    const lastCountersRef = useRef<RawCounters | null>(null);

    const parseMetrics = useCallback((payload: RawMetricsPayload): SystemMetrics => {
        const t1 = new Date(payload.timestamp).getTime();
        const currentCounters: RawCounters = {
            timestamp: t1,
            diskReadBytes: Number(payload.disk_read_bytes || 0),
            diskWriteBytes: Number(payload.disk_write_bytes || 0),
            netRecvBytes: Number(payload.net_recv_bytes || 0),
            netSentBytes: Number(payload.net_sent_bytes || 0),
        };

        let diskReadRate = 0;
        let diskWriteRate = 0;
        let netRxRate = 0;
        let netTxRate = 0;

        const prev = lastCountersRef.current;
        if (prev) {
            const t0 = prev.timestamp;
            const dt = (t1 - t0) / 1000;
            if (dt > 0) {
                diskReadRate = Math.max(0, (currentCounters.diskReadBytes - prev.diskReadBytes) / dt);
                diskWriteRate = Math.max(0, (currentCounters.diskWriteBytes - prev.diskWriteBytes) / dt);
                netRxRate = Math.max(0, (currentCounters.netRecvBytes - prev.netRecvBytes) / dt);
                netTxRate = Math.max(0, (currentCounters.netSentBytes - prev.netSentBytes) / dt);
            }
        }
        lastCountersRef.current = currentCounters;

        return {
            timestamp: t1,
            cpuLoad: payload.cpu_usage || 0,
            cpuPerCore: payload.cpu_per_core || [],
            memoryUsed: (payload.memory_used || 0) / 1024 / 1024,
            memoryTotal: (payload.memory_total || 1) / 1024 / 1024,
            swapUsed: (payload.swap_used || 0) / 1024 / 1024,
            swapTotal: (payload.swap_total || 0) / 1024 / 1024,
            diskRead: payload.disk_read_bytes ? diskReadRate / 1024 / 1024 : 0,
            diskWrite: payload.disk_write_bytes ? diskWriteRate / 1024 / 1024 : 0,
            diskIOPS: payload.disk_iops || 0,
            networkRx: payload.net_recv_bytes ? netRxRate / 1024 : 0,
            networkTx: payload.net_sent_bytes ? netTxRate / 1024 : 0,
            loadAvg1: payload.load_avg_1 || 0,
            loadAvg5: payload.load_avg_5 || 0,
            loadAvg15: payload.load_avg_15 || 0,
            temperature: payload.temperature || 0,
        };
    }, []);

    const connect = useCallback(() => {
        if (wsRef.current?.readyState === WebSocket.OPEN) return;

        try {
            const ws = new WebSocket(metricsWebSocketURL());
            wsRef.current = ws;

            ws.onopen = () => {
                setConnected(true);
                reconnectAttemptsRef.current = 0;
            };

            ws.onmessage = (event) => {
                try {
                    const msg: WSMessage = JSON.parse(event.data);
                    if (msg.type === 'metrics') {
                        const metrics = parseMetrics(msg.payload);
                        setLatestMetrics(metrics);
                        setMetricsHistory(prev => {
                            const newHistory = [...prev, metrics];
                            return newHistory.slice(-MAX_HISTORY);
                        });
                    }
                } catch (e) {
                    console.error('Failed to parse WebSocket message:', e);
                }
            };

            ws.onclose = () => {
                setConnected(false);

                // Exponential backoff reconnect
                const delay = Math.min(1000 * Math.pow(2, reconnectAttemptsRef.current), 30000);
                reconnectAttemptsRef.current++;

                reconnectTimeoutRef.current = setTimeout(() => {
                    connect();
                }, delay);
            };

            ws.onerror = (error) => {
                console.error('WebSocket error:', error);
            };
        } catch (e) {
            console.error('Failed to create WebSocket:', e);
        }
    }, [parseMetrics]);

    const reconnect = useCallback(() => {
        if (wsRef.current) {
            wsRef.current.close();
        }
        reconnectAttemptsRef.current = 0;
        connect();
    }, [connect]);

    useEffect(() => {
        connect();

        return () => {
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
            }
            if (wsRef.current) {
                wsRef.current.close();
            }
        };
    }, [connect]);

    return {
        connected,
        latestMetrics,
        metricsHistory,
        reconnect,
    };
}
