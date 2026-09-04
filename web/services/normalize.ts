// Shared defensive parsing for anything arriving from the daemon.
//
// Both transports carry the same shapes — REST (`services/api.ts`) and the
// WebSocket broadcast (`hooks/useWebSocket.ts`) — so the normalizers live here
// rather than being duplicated per transport. Divergence between the two is
// what let the process table silently disagree with the API.

import { Filesystem, Process } from '../types';

export interface RawProcess {
    cpu_core?: number;
    memory_bytes?: number;
    pid?: number;
    name?: string;
    user?: string;
    cpu?: number;
    memory?: number;
    state?: unknown;
}

export function nonEmptyString(value: unknown, fallback: string): string {
    return typeof value === 'string' && value.trim() ? value : fallback;
}

export function finiteNumber(value: unknown, fallback = 0): number {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
}

export function positiveFiniteNumber(value: unknown, fallback: number): number {
    const parsed = finiteNumber(value, fallback);
    return parsed > 0 ? parsed : fallback;
}

export function finiteNumberArray(value: unknown): number[] {
    if (!Array.isArray(value)) {
        return [];
    }
    return value.map((item) => finiteNumber(item));
}

export function normalizeProcessState(state: unknown): Process['state'] {
    if (state === 'Running' || state === 'Sleeping' || state === 'Zombie' || state === 'Stopped') {
        return state;
    }
    return 'Running';
}

export function normalizeProcess(process: RawProcess): Process {
    return {
        pid: Math.max(0, Math.trunc(finiteNumber(process.pid))),
        name: nonEmptyString(process.name, '?'),
        user: nonEmptyString(process.user, '?'),
        cpu: finiteNumber(process.cpu),
        // An agent older than these fields sends neither. Falling back keeps
        // its rows readable instead of rendering every process as 0.
        cpuCore: process.cpu_core === undefined
            ? finiteNumber(process.cpu)
            : finiteNumber(process.cpu_core),
        memory: finiteNumber(process.memory),
        memoryBytes: process.memory_bytes === undefined
            ? finiteNumber(process.memory) * 1024 * 1024
            : finiteNumber(process.memory_bytes),
        state: normalizeProcessState(process.state),
    };
}

/** Normalize a `processes` array from either transport, tolerating absence. */
export function normalizeProcesses(value: unknown): Process[] {
    if (!Array.isArray(value)) {
        return [];
    }
    return value.map((entry) => normalizeProcess((entry ?? {}) as RawProcess));
}

interface RawFilesystem {
    mountpoint?: string;
    device?: string;
    fstype?: string;
    total_bytes?: number;
    used_bytes?: number;
    free_bytes?: number;
    used_percent?: number;
    inodes_used_percent?: number;
}

export function normalizeFilesystems(value: unknown): Filesystem[] {
    if (!Array.isArray(value)) {
        return [];
    }
    return value.map((entry) => {
        const raw = (entry ?? {}) as RawFilesystem;
        return {
            mountpoint: nonEmptyString(raw.mountpoint, '?'),
            device: nonEmptyString(raw.device, ''),
            fstype: nonEmptyString(raw.fstype, ''),
            totalBytes: finiteNumber(raw.total_bytes),
            usedBytes: finiteNumber(raw.used_bytes),
            freeBytes: finiteNumber(raw.free_bytes),
            usedPercent: finiteNumber(raw.used_percent),
            inodesUsedPercent: finiteNumber(raw.inodes_used_percent),
        };
    });
}
