import assert from 'node:assert/strict';
import { afterEach, test } from 'vitest';

import {
  fetchHosts,
  fetchLatestInsight,
  fetchMe,
  fetchMetricsHistory,
  fetchSetupStatus,
  login,
  onUnauthorized,
} from './api.js';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

test('fetchLatestInsight normalizes malformed stored analysis JSON', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify([
    {
      content: JSON.stringify({
        status: 'Unknown',
        summary: '',
        recommendedActions: 'reboot now',
      }),
    },
  ]), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });

  const insight = await fetchLatestInsight();

  assert.deepEqual(insight, {
    status: 'Warning',
    summary: 'Recent Insight',
    detailedAnalysis: 'No details provided',
    recommendedActions: [],
  });
});

test('fetchMetricsHistory skips records with invalid timestamps', async () => {
  const timestamp = '2026-05-09T10:15:30Z';
  globalThis.fetch = async () => new Response(JSON.stringify([
    {
      timestamp: 'not-a-date',
      cpu_usage: 99,
      memory_total: 1024,
    },
    {
      timestamp,
      cpu_usage: 25,
      memory_used: 512,
      memory_total: 1024,
    },
  ]), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });

  const { metrics } = await fetchMetricsHistory();

  assert.equal(metrics.length, 1);
  assert.equal(metrics[0].timestamp, Date.parse(timestamp));
  assert.equal(metrics[0].cpuLoad, 25);
});

test('fetchMetricsHistory replaces non-finite metric numbers with safe defaults', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify([
    {
      timestamp: '2026-05-09T10:15:30Z',
      cpu_usage: 'hot',
      cpu_per_core: [10, 'bad', Number.NaN],
      memory_used: 'NaN',
      memory_total: 'unknown',
      disk_iops: Number.POSITIVE_INFINITY,
      load_avg_1: null,
    },
  ]), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });

  const { metrics } = await fetchMetricsHistory();

  assert.equal(metrics.length, 1);
  assert.deepEqual(metrics[0].cpuPerCore, [10, 0, 0]);
  assert.equal(metrics[0].cpuLoad, 0);
  assert.equal(metrics[0].memoryUsed, 0);
  assert.equal(metrics[0].memoryTotal, 1 / 1024 / 1024);
  assert.equal(metrics[0].diskIOPS, 0);
  assert.equal(metrics[0].loadAvg1, 0);
});

test('fetchMetricsHistory replaces non-finite process numbers with safe defaults', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify([
    {
      timestamp: '2026-05-09T10:15:30Z',
      processes: [
        {
          pid: 'bad-pid',
          name: '',
          user: '',
          cpu: 'Infinity',
          memory: 'Infinity',
          state: 'Blocked',
        },
      ],
    },
  ]), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });

  const { processes } = await fetchMetricsHistory();

  assert.deepEqual(processes, [{
    pid: 0,
    name: '?',
    user: '?',
    cpu: 0,
    cpuCore: 0,
    memory: 0,
    memoryBytes: 0,
    state: 'Running',
  }]);
});

// ---------------------------------------------------------------------------
// Auth client
// ---------------------------------------------------------------------------

test('fetchMe returns null on 401 without firing the unauthorized hook', async () => {
  let fired = 0;
  onUnauthorized(() => { fired += 1; });
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'authentication required' }), { status: 401 });
  assert.equal(await fetchMe(), null);
  assert.equal(fired, 0, '/api/auth/* 401s are an expected state, not session loss');
  onUnauthorized(null);
});

test('fetchMe returns the user and sends credentials', async () => {
  let init: RequestInit | undefined;
  globalThis.fetch = async (_input, i) => {
    init = i;
    return new Response(JSON.stringify({ user: { id: 'u1', email: 'ops@example.com', role: 'admin' } }), { status: 200 });
  };
  assert.deepEqual(await fetchMe(), { id: 'u1', email: 'ops@example.com', role: 'admin' });
  assert.equal(init?.credentials, 'same-origin');
});

test('login maps 401 and 429 to friendly errors', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'invalid email or password' }), { status: 401 });
  await assert.rejects(login('a@example.com', 'x'), /Invalid email or password/);
  globalThis.fetch = async () => new Response('', { status: 429 });
  await assert.rejects(login('a@example.com', 'x'), /Too many attempts/);
});

test('a 401 on a data route fires the unauthorized hook', async () => {
  let fired = 0;
  onUnauthorized(() => { fired += 1; });
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'authentication required' }), { status: 401 });
  await fetchSetupStatus().catch(() => undefined); // auth route: must not fire
  await fetchHosts();                              // data route: fires
  assert.equal(fired, 1);
  onUnauthorized(null);
});
