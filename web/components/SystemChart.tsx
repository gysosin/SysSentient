import React, { useMemo } from 'react';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
import { Maximize2 } from 'lucide-react';

import { SystemMetrics } from '../types';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';

/** Only numeric series are chartable. The old signature accepted
 *  `keyof SystemMetrics`, which permitted `cpuPerCore` (number[]) and
 *  `hostname` (string) as an axis. */
type NumericKey = {
  [K in keyof SystemMetrics]: SystemMetrics[K] extends number ? K : never;
}[keyof SystemMetrics];

interface SystemChartProps {
  data: SystemMetrics[];
  dataKey: NumericKey;
  color: string;
  title: string;
  unit: string;
  maxValue?: number;
}

/** Round a domain ceiling up to a readable 1/2/5 x 10^n step, so axes read
 *  "20000" rather than "20294.9609375". */
const niceCeiling = (value: number): number => {
  if (value <= 0) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const normalized = value / magnitude;
  const step = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
  return step * magnitude;
};

/** Compact large numbers on the axis: 20000 -> 20k. */
const formatTick = (value: number): string => {
  if (!Number.isFinite(value)) return '';
  const abs = Math.abs(value);
  if (abs >= 1_000_000) return `${(value / 1_000_000).toFixed(abs >= 10_000_000 ? 0 : 1)}M`;
  if (abs >= 1_000) return `${(value / 1_000).toFixed(abs >= 10_000 ? 0 : 1)}k`;
  if (abs >= 10 || Number.isInteger(value)) return value.toFixed(0);
  return value.toFixed(1);
};

const formatClock = (ms: number): string => {
  if (!Number.isFinite(ms) || ms <= 0) return '';
  const d = new Date(ms);
  return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}:${d.getSeconds().toString().padStart(2, '0')}`;
};

const SystemChart: React.FC<SystemChartProps> = ({ data, dataKey, color, title, unit, maxValue }) => {
  const gradientId = `gradient-${dataKey}`;
  const [expanded, setExpanded] = React.useState(false);

  // A flat series on an 'auto' domain renders as a wildly oscillating sawtooth,
  // which reads as instability that isn't there. Pad a floor/ceiling instead.
  const domain = useMemo<[number, number | 'auto']>(() => {
    if (maxValue) return [0, maxValue];
    const values = data.map((d) => d[dataKey]).filter((v): v is number => Number.isFinite(v));
    if (values.length === 0) return [0, 'auto'];
    const max = Math.max(...values);
    return [0, niceCeiling(max <= 0 ? 1 : max * 1.15)];
  }, [data, dataKey, maxValue]);

  const hasData = data.length > 0;

  // Hoisted so the drill-down renders the identical series at a larger size
  // rather than a second, subtly different chart.
  const chartEl = (
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={color} stopOpacity={0.35} />
              <stop offset="95%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
          {/* A metrics chart without a time axis cannot answer "when?" — the
              previous version blanked both the axis and the tooltip label. */}
          <XAxis
            dataKey="timestamp"
            type="number"
            scale="time"
            domain={['dataMin', 'dataMax']}
            tickFormatter={formatClock}
            stroke="var(--muted-foreground)"
            tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }}
            axisLine={false}
            tickLine={false}
            minTickGap={40}
          />
          <YAxis
            domain={domain}
            tickFormatter={formatTick}
            stroke="var(--muted-foreground)"
            tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }}
            width={44}
            axisLine={false}
            tickLine={false}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: 'var(--popover)',
              borderColor: 'var(--border)',
              color: 'var(--popover-foreground)',
              borderWidth: '1px',
              borderRadius: 'var(--radius-md)',
              fontSize: '12px',
            }}
            itemStyle={{ color }}
            labelStyle={{ color: 'var(--muted-foreground)' }}
            labelFormatter={(value: number) => formatClock(value)}
            formatter={(value: number) => [`${value.toFixed(1)}${unit}`, title]}
            cursor={{ stroke: 'var(--muted-foreground)', strokeWidth: 1, strokeDasharray: '4 4' }}
          />
          <Area
            type="monotone"
            dataKey={dataKey}
            stroke={color}
            fill={`url(#${gradientId})`}
            strokeWidth={2}
            // Re-animating on every 2s tick made the whole chart flicker.
            isAnimationActive={false}
            dot={false}
          />
        </AreaChart>
      </ResponsiveContainer>
  );

  return (
    <Card className="group flex h-64 flex-col">
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2">
          <span aria-hidden="true" className="size-1.5 rounded-full" style={{ backgroundColor: color }} />
          {title}
        </CardTitle>
        <span className="text-mute tabular ml-auto font-mono text-[11px]">
          {hasData ? `${data.length} samples` : 'no data'}
        </span>
        {/* Revealed on hover, and always reachable by keyboard — a control that
            only exists under a cursor is not operable by the administrators
            this is built for. */}
        <button
          type="button"
          onClick={() => setExpanded(true)}
          disabled={!hasData}
          title={`Expand ${title}`}
          aria-label={`Expand ${title}`}
          className="text-mute hover:bg-panel-strong hover:text-foreground rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100 disabled:hidden"
        >
          <Maximize2 className="size-3.5" />
        </button>
      </CardHeader>

      <CardContent className="relative flex-grow px-2 pb-3">
        {!hasData ? (
          <div className="absolute inset-0 grid place-items-center">
            <p className="text-muted-foreground text-xs">Waiting for samples…</p>
          </div>
        ) : (
          chartEl
        )}
      </CardContent>

      <Dialog open={expanded} onOpenChange={setExpanded}>
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <span
                aria-hidden="true"
                className="size-2 rounded-full"
                style={{ backgroundColor: color }}
              />
              {title}
            </DialogTitle>
          </DialogHeader>
          {/* Same data, same axes, more pixels. The point of the drill-down is
              resolution, not a different view that has to be reconciled with
              the small one. */}
          <div className="h-[55vh] min-h-[320px] w-full">{chartEl}</div>
          <p className="text-mute font-mono text-[11px]">
            {data.length} samples · {unit.trim() || 'value'} over time
          </p>
        </DialogContent>
      </Dialog>
    </Card>
  );
};

export default SystemChart;
