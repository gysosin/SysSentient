import assert from 'node:assert/strict';
import { act } from 'react';
import { renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, test, vi } from 'vitest';

import { useWebSocket } from './useWebSocket';

// The daemon broadcasts the FULL models.SystemState over the socket, including
// `processes` (internal/server/websocket.go:23). The hook used to drop that
// field, and App.tsx only populated its process list from the REST polling path
// — which is skipped whenever the socket is connected. Net effect: the process
// table was permanently empty in normal operation and only filled in when the
// WebSocket was DOWN. These tests pin the data path.

type Listener = ((event: unknown) => void) | null;

class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readyState = FakeWebSocket.CONNECTING;
  onopen: Listener = null;
  onmessage: Listener = null;
  onclose: Listener = null;
  onerror: Listener = null;

  constructor(public url: string) {
    FakeWebSocket.instances.push(this);
  }

  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.({ code: 1000 });
  }

  /** Simulate the server accepting the connection. */
  accept() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.({});
  }

  /** Simulate one broadcast frame from the daemon. */
  send(payload: unknown) {
    this.onmessage?.({ data: JSON.stringify({ type: 'metrics', payload }) });
  }
}

const frame = (overrides: Record<string, unknown> = {}) => ({
  timestamp: new Date('2026-01-01T00:00:00Z').toISOString(),
  cpu_usage: 42,
  memory_used: 8 * 1024 * 1024 * 1024,
  memory_total: 16 * 1024 * 1024 * 1024,
  processes: [
    { pid: 1234, name: 'chrome', user: 'xyfo', cpu: 12.5, memory: 512, state: 'Running' },
    { pid: 5678, name: 'node', user: 'xyfo', cpu: 4.25, memory: 128, state: 'Sleeping' },
  ],
  ...overrides,
});

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal('WebSocket', FakeWebSocket);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('useWebSocket exposes processes from the live socket', () => {
  test('processes from the broadcast payload are surfaced', () => {
    const { result } = renderHook(() => useWebSocket());
    const socket = FakeWebSocket.instances[0];
    assert.ok(socket, 'hook should have opened a socket');

    act(() => {
      socket.accept();
      socket.send(frame());
    });

    assert.equal(result.current.connected, true);
    assert.equal(
      result.current.processes.length,
      2,
      'processes must come through the WebSocket, not only the polling fallback',
    );
    assert.deepEqual(result.current.processes[0], {
      pid: 1234,
      name: 'chrome',
      user: 'xyfo',
      cpu: 12.5,
      memory: 512,
      state: 'Running',
    });
    assert.equal(result.current.processes[1].state, 'Sleeping');
  });

  test('malformed process entries are normalized, not dropped', () => {
    const { result } = renderHook(() => useWebSocket());
    const socket = FakeWebSocket.instances[0];

    act(() => {
      socket.accept();
      socket.send(
        frame({
          processes: [
            { pid: 'bad', name: '', user: undefined, cpu: Infinity, memory: null, state: 'Nonsense' },
          ],
        }),
      );
    });

    assert.deepEqual(result.current.processes[0], {
      pid: 0,
      name: '?',
      user: '?',
      cpu: 0,
      memory: 0,
      state: 'Running',
    });
  });

  test('a payload with no processes yields an empty list rather than throwing', () => {
    const { result } = renderHook(() => useWebSocket());
    const socket = FakeWebSocket.instances[0];

    act(() => {
      socket.accept();
      socket.send(frame({ processes: undefined }));
    });

    assert.deepEqual(result.current.processes, []);
    assert.equal(result.current.metricsHistory.length, 1, 'metrics must still be recorded');
  });
});
