import assert from 'node:assert/strict';
import test from 'node:test';

import { fetchLatestInsight, fetchMetricsHistory } from './api.js';

const originalFetch = globalThis.fetch;

test.afterEach(() => {
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
