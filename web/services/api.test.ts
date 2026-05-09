import assert from 'node:assert/strict';
import test from 'node:test';

import { fetchLatestInsight } from './api.js';

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
