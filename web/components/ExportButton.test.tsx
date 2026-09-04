import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';

import { ExportButton } from './ExportButton';

const exportMetrics = vi.fn(async () => undefined);

vi.mock('../services/api', () => ({
  exportMetrics: (...args: unknown[]) => exportMetrics(...(args as [never])),
}));

const dashboard = {
  range: { preset: 'live' as const },
  selectedHost: '',
  rangeResolution: 'raw',
};

vi.mock('../hooks/useDashboardData', () => ({
  useDashboard: () => dashboard,
}));

afterEach(() => {
  cleanup();
  exportMetrics.mockClear();
  dashboard.range = { preset: 'live' as const };
});

test('exports the live window without bounds, so retention decides the span', async () => {
  render(<ExportButton />);
  fireEvent.click(screen.getByText('CSV'));

  await waitFor(() => expect(exportMetrics).toHaveBeenCalledTimes(1));
  const call = exportMetrics.mock.calls[0][0] as Record<string, unknown>;
  expect(call.format).toBe('csv');
  expect(call.from).toBeUndefined();
  // Live has no resolution of its own to pin.
  expect(call.resolution).toBeUndefined();
});

test('exports exactly the selected window and tier', async () => {
  const from = new Date('2026-09-04T10:00:00Z');
  const to = new Date('2026-09-04T11:00:00Z');
  dashboard.range = { preset: 'custom', from, to } as never;

  render(<ExportButton />);
  fireEvent.click(screen.getByText('JSON'));

  await waitFor(() => expect(exportMetrics).toHaveBeenCalledTimes(1));
  const call = exportMetrics.mock.calls[0][0] as Record<string, unknown>;
  // What is exported must be what is on screen; a mismatch here means the
  // file does not describe the chart the operator was looking at.
  expect(call.format).toBe('json');
  expect(call.from).toEqual(from);
  expect(call.to).toEqual(to);
  expect(call.resolution).toBe('raw');
});

test('surfaces a failure instead of silently doing nothing', async () => {
  exportMetrics.mockRejectedValueOnce(new Error('window too large'));
  render(<ExportButton />);
  fireEvent.click(screen.getByText('CSV'));

  expect(await screen.findByRole('alert')).toHaveTextContent('window too large');
});
