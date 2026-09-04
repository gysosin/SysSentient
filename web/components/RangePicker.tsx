import { History, X } from 'lucide-react';

import { useDashboard } from '../hooks/useDashboardData';
import { RANGE_PRESETS, describeRange } from '../hooks/useTimeRange';
import { cn } from '../lib/utils';

/** How the storage tier is described to an operator. */
const RESOLUTION_LABEL: Record<string, string> = {
  raw: 'every sample',
  '1m': '1-minute averages',
  '5m': '5-minute averages',
};

/**
 * Selects the window the whole dashboard draws.
 *
 * Before this the console could only show the last two to four minutes,
 * however much history the daemon had retained — up to a year of it, reachable
 * only through an export endpoint nothing called.
 */
export function RangePicker() {
  const { range, selectRange, resetRange, rangeResolution } = useDashboard();
  const custom = range.preset === 'custom';

  return (
    <div className="flex items-center gap-1.5">
      <div
        className="border-line bg-panel flex items-center rounded-md border p-0.5"
        role="group"
        aria-label="Time range"
      >
        {RANGE_PRESETS.map((preset) => {
          const active = !custom && range.preset === preset.value;
          return (
            <button
              key={preset.value}
              type="button"
              onClick={() => selectRange(preset.value)}
              aria-pressed={active}
              className={cn(
                'rounded px-2 py-1 font-mono text-2xs transition-colors',
                active ? 'bg-brand/15 text-brand' : 'text-mute hover:text-fg',
              )}
            >
              {preset.label}
            </button>
          );
        })}
      </div>

      {/* A zoomed window is not one of the presets, so it needs its own
          affordance and a way back to live. */}
      {custom && (
        <button
          type="button"
          onClick={resetRange}
          className="border-brand/40 bg-brand/10 text-brand flex items-center gap-1.5 rounded-md border px-2 py-1 font-mono text-2xs"
          title="Return to the live feed"
        >
          <History className="size-3" aria-hidden="true" />
          {describeRange(range)}
          <X className="size-3" aria-hidden="true" />
        </button>
      )}

      {range.preset !== 'live' && (
        <span className="text-mute hidden font-mono text-2xs lg:inline">
          {RESOLUTION_LABEL[rangeResolution] ?? rangeResolution}
        </span>
      )}
    </div>
  );
}
