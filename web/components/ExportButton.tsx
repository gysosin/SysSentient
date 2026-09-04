import { useState } from 'react';
import { Check, Download, Loader2 } from 'lucide-react';

import { exportMetrics } from '../services/api';
import { useDashboard } from '../hooks/useDashboardData';
import { resolveBounds } from '../hooks/useTimeRange';
import { Button } from './ui/button';
import { cn } from '../lib/utils';

/**
 * Downloads the current window.
 *
 * The export endpoint has been complete server-side since the backup work and
 * no part of the UI called it, so getting data out meant constructing a URL by
 * hand. What is exported is exactly what is on screen: the selected window, the
 * selected host, and the tier that answered it.
 */
export function ExportButton({ className }: { className?: string }) {
  const { range, selectedHost, rangeResolution } = useDashboard();
  const [state, setState] = useState<'idle' | 'working' | 'done' | 'failed'>('idle');
  const [error, setError] = useState<string | null>(null);

  const run = async (format: 'csv' | 'json') => {
    setState('working');
    setError(null);
    try {
      // Live has no bounds of its own; export what the retention holds up to
      // now rather than refusing.
      const bounds = resolveBounds(range);
      await exportMetrics({
        format,
        from: bounds?.from,
        to: bounds?.to,
        hostID: selectedHost,
        resolution: range.preset === 'live' ? undefined : rangeResolution,
      });
      setState('done');
    } catch (err) {
      setState('failed');
      setError(err instanceof Error ? err.message : 'Export failed');
    }
    window.setTimeout(() => setState('idle'), 2500);
  };

  return (
    <div className={cn('flex items-center gap-1.5', className)}>
      <Button
        variant="outline"
        size="sm"
        onClick={() => void run('csv')}
        disabled={state === 'working'}
        title="Download the current window as CSV"
      >
        {state === 'working' ? (
          <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
        ) : state === 'done' ? (
          <Check className="size-3.5" aria-hidden="true" />
        ) : (
          <Download className="size-3.5" aria-hidden="true" />
        )}
        CSV
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={() => void run('json')}
        disabled={state === 'working'}
        title="Download the current window as JSON"
      >
        JSON
      </Button>
      {error && (
        <span role="alert" className="text-crit text-2xs">
          {error}
        </span>
      )}
    </div>
  );
}
