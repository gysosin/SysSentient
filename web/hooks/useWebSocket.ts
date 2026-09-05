import { useState, useEffect, useCallback, useRef } from 'react';
import { Process, SystemMetrics } from '../types';
import { normalizeFilesystems, normalizeProcesses } from '../services/normalize';
import { metricsWebSocketURL } from '../constants';

interface RawMetricsPayload {
    process_count?: number;
    host_id?: string;
    hostname?: string;
    timestamp: string;
    cpu_usage?: number;
    cpu_per_core?: number[];
    memory_used?: number;
    memory_total?: number;
    memory_cached?: number;
    memory_buffers?: number;
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
    uptime_seconds?: number;
    // Broadcast by the daemon (internal/server/websocket.go:23 sends the full
    // SystemState). Previously omitted here, so the process table stayed empty
    // whenever the socket was up.
    processes?: unknown;
    filesystems?: unknown;
}

type WSMessage = {
    type: 'metrics';
    payload: RawMetricsPayload;
} | {
    type: 'alert' | 'prediction';
    payload: unknown;
}

function isMetricsMessage(value: unknown): value is Extract<WSMessage, { type: 'metrics' }> {
    if (!value || typeof value !== 'object') return false;
    const message = value as Record<string, unknown>;
    if (message.type !== 'metrics' || !message.payload || typeof message.payload !== 'object') {
        return false;
    }
    const payload = message.payload as Record<string, unknown>;
    return typeof payload.timestamp === 'string' && Number.isFinite(Date.parse(payload.timestamp));
}

interface UseWebSocketReturn {
    connected: boolean;
    metricsHistory: SystemMetrics[];
    processes: Process[];
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

/**
 * Streams metrics over the socket while `enabled` is true.
 *
 * `enabled` is page visibility. Chrome throttles timers in a hidden tab but not
 * WebSocket messages, so without this a background tab kept receiving a frame
 * every two seconds and re-rendering the whole application for it. Closing the
 * socket rather than ignoring frames also stops the server encoding and sending
 * a frame to this client at all.
 */
export function useWebSocket(enabled = true): UseWebSocketReturn {
    const [connected, setConnected] = useState(false);
    const [metricsHistory, setMetricsHistory] = useState<SystemMetrics[]>([]);
    const [processes, setProcesses] = useState<Process[]>([]);
    const wsRef = useRef<WebSocket | null>(null);
    const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
    const reconnectAttemptsRef = useRef(0);
    const lastCountersRef = useRef<RawCounters | null>(null);
    const shouldReconnectRef = useRef(true);

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
            hostname: payload.hostname || '',
            timestamp: t1,
            cpuLoad: payload.cpu_usage || 0,
            cpuPerCore: payload.cpu_per_core || [],
            memoryUsed: (payload.memory_used || 0) / 1024 / 1024,
            memoryTotal: (payload.memory_total || 1) / 1024 / 1024,
            memoryCached: (payload.memory_cached || 0) / 1024 / 1024,
            memoryBuffers: (payload.memory_buffers || 0) / 1024 / 1024,
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
            uptimeSeconds: payload.uptime_seconds || 0,
            processCount: payload.process_count || 0,
            hostId: payload.host_id || '',
            filesystems: normalizeFilesystems(payload.filesystems),
        };
    }, []);

    const connect = useCallback(() => {
        if (wsRef.current?.readyState === WebSocket.OPEN) return;
        shouldReconnectRef.current = true;

        try {
            const ws = new WebSocket(metricsWebSocketURL());
            wsRef.current = ws;

            ws.onopen = () => {
                setConnected(true);
                reconnectAttemptsRef.current = 0;
            };

            ws.onmessage = (event) => {
                try {
                    const msg = JSON.parse(event.data) as unknown;
                    if (isMetricsMessage(msg)) {
                        const metrics = parseMetrics(msg.payload);
                        setProcesses(normalizeProcesses(msg.payload.processes));
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
                if (!shouldReconnectRef.current) {
                    return;
                }

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
        shouldReconnectRef.current = true;
        if (wsRef.current) {
            wsRef.current.close();
        }
        reconnectAttemptsRef.current = 0;
        connect();
    }, [connect]);

    useEffect(() => {
        if (!enabled) {
            // Hidden: close and stay closed. The cleanup below has already run
            // for the previous (enabled) pass, so this is belt and braces for
            // the case where the effect first mounts hidden.
            shouldReconnectRef.current = false;
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
            }
            wsRef.current?.close();
            setConnected(false);
            return;
        }

        // Shown again: reconnect now, not after whatever backoff the last
        // disconnect left behind -- the operator is looking at the screen.
        reconnectAttemptsRef.current = 0;
        connect();

        return () => {
            shouldReconnectRef.current = false;
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
            }
            if (wsRef.current) {
                wsRef.current.close();
            }
        };
    }, [connect, enabled]);

    return {
        connected,
        metricsHistory,
        processes,
        reconnect,
    };
}
