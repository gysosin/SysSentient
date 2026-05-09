import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import AIInsightPanel from './AIInsightPanel.js';
import { AIAnalysisResult } from '../types.js';

const analysis: AIAnalysisResult = {
  status: 'Warning',
  summary: 'Memory pressure detected',
  detailedAnalysis: 'Swap usage is increasing across recent samples.',
  recommendedActions: [
    {
      id: 'action-restart',
      command: 'systemctl restart cache.service',
      description: 'Restart the cache worker after confirming active requests drained.',
      isSafe: false,
    },
  ],
};

function renderAnalysisID(): string {
  const html = renderToStaticMarkup(
    <AIInsightPanel analysis={analysis} error={null} loading={false} onRefresh={() => undefined} />,
  );
  const match = html.match(/ID: ([A-Z0-9]+)/);
  assert.ok(match, `expected rendered AI panel to include an analysis ID: ${html}`);
  return match[1];
}

test('AIInsightPanel renders a stable display ID for the same analysis payload', () => {
  const ids = Array.from({ length: 5 }, renderAnalysisID);

  assert.equal(new Set(ids).size, 1, `expected a stable ID, got ${ids.join(', ')}`);
});
