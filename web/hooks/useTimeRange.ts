import { useCallback, useMemo, useState } from 'react';

/**
 * A dashboard time window.
 *
 * `live` is the streaming tail the console has always shown. Every other preset
 * is a bounded query answered from the storage tier that fits the span, which
 * is what makes a year of retained history reachable at all — before this the
 * dashboard could only display the last two to four minutes of it.
 */
export type RangePreset = 'live' | '15m' | '1h' | '6h' | '24h' | '7d' | '30d' | 'custom';

export interface TimeRange {
  preset: RangePreset;
  /** Absolute bounds. Undefined while `preset` is 'live'. */
  from?: Date;
  to?: Date;
}

/** Span of each preset, used to derive absolute bounds at query time. */
const PRESET_SPANS: Record<Exclude<RangePreset, 'live' | 'custom'>, number> = {
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
  '30d': 30 * 24 * 60 * 60 * 1000,
};

export const RANGE_PRESETS: { value: RangePreset; label: string }[] = [
  { value: 'live', label: 'Live' },
  { value: '15m', label: '15m' },
  { value: '1h', label: '1h' },
  { value: '6h', label: '6h' },
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
];

/** Resolves a range to absolute bounds, or null while live. */
export function resolveBounds(range: TimeRange, now = Date.now()): { from: Date; to: Date } | null {
  if (range.preset === 'live') return null;
  if (range.preset === 'custom') {
    return range.from && range.to ? { from: range.from, to: range.to } : null;
  }
  const span = PRESET_SPANS[range.preset];
  return { from: new Date(now - span), to: new Date(now) };
}

/** Human label for the current window, for headers and export filenames. */
export function describeRange(range: TimeRange): string {
  if (range.preset === 'live') return 'Live';
  if (range.preset !== 'custom') {
    return RANGE_PRESETS.find((p) => p.value === range.preset)?.label ?? range.preset;
  }
  if (!range.from || !range.to) return 'Custom';
  const minutes = Math.round((range.to.getTime() - range.from.getTime()) / 60000);
  if (minutes < 60) return `${minutes}m window`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `${hours}h window`;
  return `${Math.round(hours / 24)}d window`;
}

/**
 * Holds the dashboard's window.
 *
 * Zooming into a chart produces a `custom` range, so dragging over a spike and
 * reading the processes from that exact minute is the same mechanism as
 * picking "last 6 hours" — one window, respected everywhere.
 */
export function useTimeRange(initial: RangePreset = 'live') {
  const [range, setRange] = useState<TimeRange>({ preset: initial });

  const selectPreset = useCallback((preset: RangePreset) => {
    setRange({ preset });
  }, []);

  const zoomTo = useCallback((from: Date, to: Date) => {
    // A drag that lands on a single point is a click, not a zoom; zooming to a
    // zero-width window would blank every chart.
    if (to.getTime() - from.getTime() < 1000) return;
    setRange({ preset: 'custom', from, to });
  }, []);

  const reset = useCallback(() => setRange({ preset: 'live' }), []);

  return useMemo(
    () => ({ range, selectPreset, zoomTo, reset, isLive: range.preset === 'live' }),
    [range, selectPreset, zoomTo, reset],
  );
}
