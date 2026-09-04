import { describe, expect, it } from 'vitest';

import { describeRange, resolveBounds } from './useTimeRange';

describe('resolveBounds', () => {
  it('returns nothing while live, so the socket keeps driving the view', () => {
    expect(resolveBounds({ preset: 'live' })).toBeNull();
  });

  it('derives bounds from a preset span', () => {
    const now = Date.parse('2026-09-04T12:00:00Z');
    const bounds = resolveBounds({ preset: '6h' }, now);
    expect(bounds).not.toBeNull();
    expect(bounds!.to.getTime()).toBe(now);
    expect(bounds!.from.getTime()).toBe(now - 6 * 60 * 60 * 1000);
  });

  it('uses the explicit bounds of a custom window', () => {
    const from = new Date('2026-09-04T10:00:00Z');
    const to = new Date('2026-09-04T11:00:00Z');
    expect(resolveBounds({ preset: 'custom', from, to })).toEqual({ from, to });
  });

  it('treats an incomplete custom window as live rather than querying nothing', () => {
    expect(resolveBounds({ preset: 'custom', from: new Date() })).toBeNull();
  });
});

describe('describeRange', () => {
  it('labels presets and custom windows', () => {
    expect(describeRange({ preset: 'live' })).toBe('Live');
    expect(describeRange({ preset: '24h' })).toBe('24h');
    expect(
      describeRange({
        preset: 'custom',
        from: new Date('2026-09-04T10:00:00Z'),
        to: new Date('2026-09-04T10:30:00Z'),
      }),
    ).toBe('30m window');
  });
});
