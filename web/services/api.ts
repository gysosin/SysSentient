import type { FleetHost, HealthStatus, Alert, AlertEvent, AlertRule, SystemMetrics, AIAction, AIAnalysisResult, Process, LogEntry } from '../types';
import { API_BASE_URL } from '../constants.js';
import {
    RawProcess,
    finiteNumber,
    finiteNumberArray,
    nonEmptyString,
    normalizeFilesystems,
    normalizeProcess,
    positiveFiniteNumber,
} from './normalize';

const API_REQUEST_TIMEOUT_MS = 8000;

interface LogsResponse {
    collectedAt?: string;
    content?: string;
}

interface ErrorResponse {
    error?: string;
}

interface RawSystemState {
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
    temperature?: number;
    uptime_seconds?: number;
    disk_read_bytes?: number;
    disk_write_bytes?: number;
    disk_iops?: number;
    net_recv_bytes?: number;
    net_sent_bytes?: number;
    load_avg_1?: number;
    load_avg_5?: number;
    load_avg_15?: number;
    processes?: RawProcess[];
    filesystems?: unknown;
    top_processes?: string;
}

interface InsightRecord {
    content?: string;
}

const AI_STATUSES = new Set<AIAnalysisResult['status']>(['Healthy', 'Warning', 'Critical']);

export const fetchMetricsHistory = async (hostID = ''): Promise<{ metrics: SystemMetrics[], processes: Process[] }> => {
    try {
        const query = hostID ? `?host=${encodeURIComponent(hostID)}` : '';
        const response = await fetchWithTimeout(`${API_BASE_URL}/metrics${query}`);
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
                hostname: nonEmptyString(curr.hostname, ''),
                timestamp: t1,
                cpuLoad: finiteNumber(curr.cpu_usage),
                cpuPerCore: finiteNumberArray(curr.cpu_per_core),
                memoryUsed: finiteNumber(curr.memory_used) / 1024 / 1024,
                memoryTotal: positiveFiniteNumber(curr.memory_total, 1) / 1024 / 1024,
                memoryCached: finiteNumber(curr.memory_cached) / 1024 / 1024,
                memoryBuffers: finiteNumber(curr.memory_buffers) / 1024 / 1024,
                swapUsed: finiteNumber(curr.swap_used) / 1024 / 1024,
                swapTotal: finiteNumber(curr.swap_total) / 1024 / 1024,
                temperature: finiteNumber(curr.temperature),
                uptimeSeconds: finiteNumber(curr.uptime_seconds),
                filesystems: normalizeFilesystems(curr.filesystems),
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
        const response = await fetchWithTimeout(`${API_BASE_URL}/insights`);
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
        const response = await fetchWithTimeout(`${API_BASE_URL}/logs`);
        if (!response.ok) throw new Error('Failed to fetch logs');

        const data = await response.json() as LogsResponse;
        return parseLogContent(data.content || '', data.collectedAt || new Date().toISOString());
    } catch (e) {
        console.error("API Error fetchRecentLogs", e);
        return [];
    }
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

export class UnauthorizedError extends Error {
    constructor() {
        super('Not authenticated');
        this.name = 'UnauthorizedError';
    }
}

let unauthorizedHandler: (() => void) | null = null;

/**
 * AuthProvider registers here so that any *data* request coming back 401 drops
 * the app to the login screen, instead of rendering a dashboard of zeros
 * forever the way the old build did.
 */
export function onUnauthorized(handler: (() => void) | null): void {
    unauthorizedHandler = handler;
}

// A 401 from /api/auth/* is an expected answer ("you are not signed in"), not
// the loss of a session, so it must not bounce the user anywhere.
const isAuthRoute = (input: RequestInfo | URL): boolean => String(input).includes('/api/auth/');

async function fetchWithTimeout(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const controller = new AbortController();
    const timeoutID = setTimeout(() => controller.abort(), API_REQUEST_TIMEOUT_MS);

    try {
        const response = await fetch(input, {
            credentials: 'same-origin',
            ...init,
            signal: controller.signal,
        });
        if (response.status === 401 && !isAuthRoute(input)) {
            unauthorizedHandler?.();
        }
        return response;
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

export const fetchActiveAlerts = async (hostID = ''): Promise<Alert[]> => {
    try {
        const query = hostID ? `?host=${encodeURIComponent(hostID)}` : '';
        const response = await fetchWithTimeout(`${API_BASE_URL}/alerts${query}`);
        if (!response.ok) throw new Error('Failed to fetch alerts');
        const raw = await response.json() as unknown;
        if (!Array.isArray(raw)) return [];
        return raw.map((entry) => {
            const a = (entry ?? {}) as Record<string, unknown>;
            return {
                ruleId: nonEmptyString(a.rule_id, '?'),
                ruleName: nonEmptyString(a.rule_name, '?'),
                metric: nonEmptyString(a.metric, ''),
                state: (a.state === 'pending' || a.state === 'firing' || a.state === 'resolved') ? a.state : 'pending',
                severity: a.severity === 'critical' ? 'critical' : 'warning',
                value: finiteNumber(a.value),
                threshold: finiteNumber(a.threshold),
                hostname: nonEmptyString(a.hostname, ''),
                startedAt: nonEmptyString(a.started_at, ''),
                acknowledged: a.acknowledged === true,
            } as Alert;
        });
    } catch (e) {
        console.error('API Error fetchActiveAlerts', e);
        return [];
    }
};

export const fetchAlertRules = async (): Promise<AlertRule[]> => {
    try {
        const response = await fetchWithTimeout(`${API_BASE_URL}/alerts/rules`);
        if (!response.ok) throw new Error('Failed to fetch alert rules');
        const raw = await response.json() as unknown;
        if (!Array.isArray(raw)) return [];
        return raw.map((entry) => {
            const r = (entry ?? {}) as Record<string, unknown>;
            return {
                id: nonEmptyString(r.id, '?'),
                name: nonEmptyString(r.name, '?'),
                metric: nonEmptyString(r.metric, ''),
                op: nonEmptyString(r.op, '>'),
                threshold: finiteNumber(r.threshold),
                forLabel: nonEmptyString(r.for_label, '0s'),
                severity: r.severity === 'critical' ? 'critical' : 'warning',
                enabled: r.enabled !== false,
            } as AlertRule;
        });
    } catch (e) {
        console.error('API Error fetchAlertRules', e);
        return [];
    }
};

export const fetchAlertHistory = async (limit = 50): Promise<AlertEvent[]> => {
    try {
        const response = await fetchWithTimeout(`${API_BASE_URL}/alerts/history?limit=${limit}`);
        if (!response.ok) throw new Error('Failed to fetch alert history');
        const raw = await response.json() as unknown;
        if (!Array.isArray(raw)) return [];
        return raw.map((entry) => {
            const e = (entry ?? {}) as Record<string, unknown>;
            return {
                occurredAt: nonEmptyString(e.occurred_at, ''),
                ruleId: nonEmptyString(e.rule_id, '?'),
                ruleName: nonEmptyString(e.rule_name, '?'),
                metric: nonEmptyString(e.metric, ''),
                state: (e.state === 'pending' || e.state === 'firing' || e.state === 'resolved') ? e.state : 'firing',
                severity: e.severity === 'critical' ? 'critical' : 'warning',
                value: finiteNumber(e.value),
                threshold: finiteNumber(e.threshold),
                hostname: nonEmptyString(e.hostname, ''),
            } as AlertEvent;
        });
    } catch (e) {
        console.error('API Error fetchAlertHistory', e);
        return [];
    }
};

export const acknowledgeAlert = async (ruleId: string, hostID = ''): Promise<void> => {
    const query = hostID ? `?host=${encodeURIComponent(hostID)}` : '';
    const response = await fetchWithTimeout(`${API_BASE_URL}/alerts/${encodeURIComponent(ruleId)}/acknowledge${query}`, {
        method: 'POST',
    });
    if (!response.ok) {
        throw new Error(await readAPIError(response, 'Failed to acknowledge alert'));
    }
};

// /health is unauthenticated and lives outside the /api prefix.
const HEALTH_URL = API_BASE_URL.replace(/\/api\/?$/, '') + '/health';

export const fetchHealth = async (): Promise<HealthStatus | null> => {
    try {
        // A degraded daemon answers 503 with a valid body, so the response is
        // parsed regardless of status rather than treated as a failure.
        const response = await fetchWithTimeout(HEALTH_URL);
        const h = await response.json() as Record<string, unknown>;
        return {
            status: nonEmptyString(h.status, 'unknown'),
            service: nonEmptyString(h.service, 'sys-sentient'),
            database: nonEmptyString(h.database, 'unknown'),
            version: nonEmptyString(h.version, ''),
            commit: nonEmptyString(h.commit, ''),
            collector: nonEmptyString(h.collector, ''),
            lastSampleAgeSeconds: typeof h.last_sample_age_seconds === 'number' ? h.last_sample_age_seconds : undefined,
            websocketClients: typeof h.websocket_clients === 'number' ? h.websocket_clients : undefined,
        };
    } catch (e) {
        console.error('API Error fetchHealth', e);
        return null;
    }
};

export const fetchHosts = async (): Promise<FleetHost[]> => {
    try {
        const response = await fetchWithTimeout(`${API_BASE_URL}/hosts`);
        if (!response.ok) throw new Error('Failed to fetch hosts');
        const raw = await response.json() as unknown;
        if (!Array.isArray(raw)) return [];
        return raw.map((entry) => {
            const h = (entry ?? {}) as Record<string, unknown>;
            return {
                hostId: nonEmptyString(h.host_id, ''),
                hostname: nonEmptyString(h.hostname, 'unknown'),
                firstSeen: nonEmptyString(h.first_seen, ''),
                lastSeen: nonEmptyString(h.last_seen, ''),
                agentVersion: nonEmptyString(h.agent_version, ''),
            } as FleetHost;
        }).filter((h) => h.hostId !== '');
    } catch (e) {
        console.error('API Error fetchHosts', e);
        return [];
    }
};

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

export interface AuthUser {
    id: string;
    email: string;
    role: 'admin' | 'viewer';
}

export interface ManagedUser extends AuthUser {
    createdAt: string;
    lastLoginAt: string | null;
}

const JSON_HEADERS = { 'Content-Type': 'application/json' };

function asAuthUser(payload: unknown): AuthUser {
    const record = asRecord(payload);
    const user = asRecord(record?.user) ?? record;
    return {
        id: nonEmptyString(user?.id, ''),
        email: nonEmptyString(user?.email, ''),
        role: user?.role === 'admin' ? 'admin' : 'viewer',
    };
}

/** Resolves the current session. null means "not signed in", not an error. */
export const fetchMe = async (): Promise<AuthUser | null> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/me`);
    if (response.status === 401) return null;
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to load session'));
    return asAuthUser(await response.json());
};

/** True while the daemon has no accounts and is waiting for first-run setup. */
export const fetchSetupStatus = async (): Promise<boolean> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/setup`);
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to check setup state'));
    return asRecord(await response.json())?.needsSetup === true;
};

export const login = async (email: string, password: string): Promise<AuthUser> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/login`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ email, password }),
    });
    if (response.status === 401) throw new Error('Invalid email or password');
    if (response.status === 429) throw new Error('Too many attempts. Try again in a minute.');
    if (!response.ok) throw new Error(await readAPIError(response, 'Sign-in failed'));
    return asAuthUser(await response.json());
};

export const logout = async (): Promise<void> => {
    await fetchWithTimeout(`${API_BASE_URL}/auth/logout`, { method: 'POST' });
};

export const completeSetup = async (token: string, email: string, password: string): Promise<AuthUser> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/setup`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ token, email, password }),
    });
    if (response.status === 403) throw new Error('That setup token is not valid. Check the daemon log.');
    if (response.status === 409) throw new Error('Setup has already been completed. Sign in instead.');
    if (!response.ok) throw new Error(await readAPIError(response, 'Setup failed'));
    return asAuthUser(await response.json());
};

export const changePassword = async (currentPassword: string, newPassword: string): Promise<void> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/password`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ currentPassword, newPassword }),
    });
    if (!response.ok) throw new Error(await readAPIError(response, 'Password change failed'));
};

function asManagedUser(payload: unknown): ManagedUser {
    const record = asRecord(payload);
    return {
        ...asAuthUser(record),
        createdAt: nonEmptyString(record?.createdAt, ''),
        lastLoginAt: typeof record?.lastLoginAt === 'string' ? record.lastLoginAt : null,
    };
}

export const fetchUsers = async (): Promise<ManagedUser[]> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/users`);
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to load users'));
    const payload = await response.json();
    return Array.isArray(payload) ? payload.map(asManagedUser) : [];
};

export const createUser = async (email: string, password: string, role: AuthUser['role']): Promise<ManagedUser> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/users`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ email, password, role }),
    });
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to create user'));
    return asManagedUser(await response.json());
};

export const deleteUser = async (id: string): Promise<void> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/users/${encodeURIComponent(id)}`, { method: 'DELETE' });
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to delete user'));
};

/** Settings that can be changed without restarting the daemon. */
export interface RuntimeSettings {
  poll_interval_seconds: number;
  metrics_retention_hours: number;
  minute_rollup_days: number;
  five_minute_rollup_days: number;
  log_level: string;
}

export async function fetchRuntimeSettings(): Promise<RuntimeSettings | null> {
  try {
    const res = await fetchWithTimeout(`${API_BASE_URL}/settings`);
    if (!res.ok) return null;
    return (await res.json()) as RuntimeSettings;
  } catch {
    return null;
  }
}

/**
 * Applies a partial settings change.
 *
 * Sends only the changed fields: the server treats absent keys as "leave
 * alone", so posting the whole object would make two admins editing different
 * settings overwrite each other.
 */
export async function updateRuntimeSettings(
  patch: Partial<RuntimeSettings>,
): Promise<RuntimeSettings> {
  const res = await fetchWithTimeout(`${API_BASE_URL}/settings`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
  if (!res.ok) {
    // The server's validation messages name the value and its bounds, which is
    // exactly what the operator needs to see.
    throw new Error((await res.text()).trim() || 'Failed to update settings');
  }
  return (await res.json()) as RuntimeSettings;
}

/** An enrolled machine reporting to this server. */
export interface FleetAgent {
  id: string;
  host_id: string;
  hostname: string;
  label: string;
  created_at: string;
  last_seen_at?: string;
  agent_version: string;
  revoked_at?: string;
}

/** A single-use invitation awaiting redemption. */
export interface JoinToken {
  id: string;
  label: string;
  created_at: string;
  expires_at: string;
  created_by: string;
}

/**
 * A freshly minted token.
 *
 * `token` is returned exactly once, at creation — the server stores only its
 * hash — so the UI must show it immediately and cannot offer to reveal it later.
 */
export interface IssuedJoinToken {
  token: string;
  id: string;
  label: string;
  expires_at: string;
  command: string;
}

export async function fetchAgents(): Promise<FleetAgent[]> {
  const res = await fetchWithTimeout(`${API_BASE_URL}/agents`);
  if (!res.ok) throw new Error('Failed to load devices');
  const body = (await res.json()) as { agents: FleetAgent[] | null };
  return body.agents ?? [];
}

export async function fetchJoinTokens(): Promise<JoinToken[]> {
  const res = await fetchWithTimeout(`${API_BASE_URL}/agents/tokens`);
  if (!res.ok) throw new Error('Failed to load pending invitations');
  const body = (await res.json()) as { tokens: JoinToken[] | null };
  return body.tokens ?? [];
}

export async function createJoinToken(label: string): Promise<IssuedJoinToken> {
  const res = await fetchWithTimeout(`${API_BASE_URL}/agents/tokens`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ label }),
  });
  if (!res.ok) {
    throw new Error((await res.text()).trim() || 'Failed to create a join token');
  }
  return (await res.json()) as IssuedJoinToken;
}

export async function revokeAgent(id: string): Promise<void> {
  const res = await fetchWithTimeout(`${API_BASE_URL}/agents/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
  if (!res.ok) {
    throw new Error((await res.text()).trim() || 'Failed to revoke this device');
  }
}
