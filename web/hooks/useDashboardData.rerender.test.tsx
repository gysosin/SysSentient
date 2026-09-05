import { act, render } from '@testing-library/react';
import React from 'react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

// Every network call is stubbed: this test is about render counts under a
// clock, not about data.
// Every fetch returns the SAME object every time. That is what isolates the
// clock: React bails out of a state update when the new value is reference-
// equal to the old one, so any render counted below came from the tick and not
// from a poll delivering a fresh (if identical) array.
const NO_METRICS: never[] = [];
const NO_PROCESSES: never[] = [];
const NO_LOGS: never[] = [];
const NO_INSIGHTS: never[] = [];
const NO_ALERTS: never[] = [];
const NO_HOSTS: never[] = [];
const STABLE_HISTORY = { metrics: NO_METRICS, processes: NO_PROCESSES };

vi.mock('../services/api', () => ({
  fetchMetricsHistory: vi.fn(async () => STABLE_HISTORY),
  fetchMetricsRange: vi.fn(async () => null),
  fetchRecentLogs: vi.fn(async () => NO_LOGS),
  fetchInsightHistory: vi.fn(async () => NO_INSIGHTS),
  fetchActiveAlerts: vi.fn(async () => NO_ALERTS),
  fetchHosts: vi.fn(async () => NO_HOSTS),
  triggerAnalysis: vi.fn(async () => {
    throw new Error('not used');
  }),
}));

class SilentWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  readyState = SilentWebSocket.CONNECTING;
  onopen: unknown = null;
  onmessage: unknown = null;
  onclose: unknown = null;
  onerror: unknown = null;
  constructor(public url: string) {}
  close() {
    this.readyState = SilentWebSocket.CLOSED;
  }
}

beforeEach(() => {
  vi.stubGlobal('WebSocket', SilentWebSocket);
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

/**
 * Counts how many times a consumer that does NOT read the feed is re-rendered.
 *
 * The provider ticks a clock every second to drive the "updated Ns ago"
 * readout. That tick used to sit in the context value's dependency list, so a
 * new context object was published every second and React re-rendered every
 * consumer — eleven of them, including four charts — for a string only the feed
 * chip displays.
 */
async function countRendersOverSeconds(seconds: number): Promise<number> {
  const { DashboardProvider, useDashboard } = await import('./useDashboardData');

  let renders = 0;
  const Consumer: React.FC = () => {
    // Deliberately reads something that does not change on a clock tick.
    const { hosts } = useDashboard();
    renders++;
    return <span>{hosts.length}</span>;
  };

  render(
    <DashboardProvider>
      <Consumer />
    </DashboardProvider>,
  );

  // Let mount effects settle before counting steady-state renders.
  await act(async () => {
    await vi.advanceTimersByTimeAsync(50);
  });
  const afterMount = renders;

  // One second at a time, each in its own act(). Advancing the whole span in a
  // single act() lets React batch every tick into one render, which would hide
  // exactly the behaviour under test.
  for (let i = 0; i < seconds; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
  }

  return renders - afterMount;
}

test('a consumer that ignores the feed is not re-rendered by the clock tick', async () => {
  const renders = await countRendersOverSeconds(10);

  // Ten seconds of wall clock with no new data, no socket frames and no
  // polling results. A consumer reading only `hosts` has nothing to redraw.
  // Before this was fixed the count was one per second.
  expect(renders).toBeLessThanOrEqual(2);
});

test('a consumer that reads the feed still updates every second', async () => {
  const { DashboardProvider, useFeed } = await import('./useDashboardData');

  let renders = 0;
  const FeedConsumer: React.FC = () => {
    const feed = useFeed();
    renders++;
    return <span>{feed.label}</span>;
  };

  render(
    <DashboardProvider>
      <FeedConsumer />
    </DashboardProvider>,
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(50);
  });
  const afterMount = renders;

  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
  }

  // The whole point of splitting the context was to keep this working while
  // everything else stopped re-rendering. A "last updated" readout that stops
  // counting is worse than no readout at all.
  expect(renders - afterMount).toBeGreaterThanOrEqual(4);
});

test('no page subscribes to the feed at page level', async () => {
  // A page that calls useFeed() directly re-renders once a second, taking its
  // whole subtree with it. The feed belongs in small leaf components; this
  // catches a regression that would otherwise only show up in a profiler.
  const { readFile } = await import('node:fs/promises');
  const { resolve } = await import('node:path');
  const pages = [
    'pages/Overview.tsx',
    'pages/Processes.tsx',
    'pages/Logs.tsx',
    'pages/Settings.tsx',
  ];

  const offenders: string[] = [];
  for (const page of pages) {
    // vitest runs with the web/ directory as its working directory.
    const src = await readFile(resolve(process.cwd(), page), 'utf8');
    // The page component itself, everything after its declaration.
    const decl = src.search(/^const (Overview|Processes|Logs|Settings): React\.FC/m);
    if (decl === -1) continue;
    if (/\buseFeed\(\)/.test(src.slice(decl))) offenders.push(page);
  }

  expect(offenders).toEqual([]);
});
