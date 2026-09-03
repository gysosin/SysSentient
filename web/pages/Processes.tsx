import React, { useMemo, useState } from 'react';
import { motion } from 'motion/react';
import { ArrowDown, ArrowUp, Search } from 'lucide-react';

import { Process } from '../types';
import { useDashboard } from '../hooks/useDashboardData';
import { Card, CardContent, CardHeader } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Badge } from '../components/ui/badge';
import { Meter } from '../components/ui/motion-primitives';
import { ScreenHeading } from '../components/ui/section-heading';
import { cn } from '../lib/utils';

type SortKey = 'pid' | 'name' | 'user' | 'cpu' | 'memory' | 'state';

const COLUMNS: { key: SortKey; label: string; numeric?: boolean; className?: string }[] = [
  { key: 'pid', label: 'PID', numeric: true, className: 'w-24' },
  { key: 'name', label: 'Process' },
  { key: 'user', label: 'User', className: 'w-32' },
  { key: 'cpu', label: 'CPU', numeric: true, className: 'w-44' },
  { key: 'memory', label: 'Memory', numeric: true, className: 'w-28' },
  { key: 'state', label: 'State', className: 'w-28' },
];

const stateVariant = (state: Process['state']) =>
  state === 'Zombie' ? 'crit' : state === 'Running' ? 'ok' : 'outline';

const Processes: React.FC = () => {
  const { processes, feed } = useDashboard();
  const [query, setQuery] = useState('');
  const [sortKey, setSortKey] = useState<SortKey>('cpu');
  const [descending, setDescending] = useState(true);

  const peakCPU = useMemo(
    () => Math.max(100, ...processes.map((p) => p.cpu)),
    [processes],
  );

  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const filtered = needle
      ? processes.filter(
          (p) =>
            p.name.toLowerCase().includes(needle) ||
            p.user.toLowerCase().includes(needle) ||
            String(p.pid).includes(needle),
        )
      : processes;

    return [...filtered].sort((a, b) => {
      const av = a[sortKey];
      const bv = b[sortKey];
      const cmp =
        typeof av === 'number' && typeof bv === 'number'
          ? av - bv
          : String(av).localeCompare(String(bv));
      return descending ? -cmp : cmp;
    });
  }, [processes, query, sortKey, descending]);

  const toggleSort = (key: SortKey) => {
    if (key === sortKey) {
      setDescending((d) => !d);
    } else {
      setSortKey(key);
      setDescending(true);
    }
  };

  const dimmed = feed.level === 'stale' || feed.level === 'down';

  return (
    <>
      <ScreenHeading
        eyebrow="Runtime inventory"
        title="Processes"
        description="What is consuming the machine right now. Sort, filter and search without losing the host context."
      />

      <Card className={cn(dimmed && 'opacity-60')}>
      <CardHeader className="flex-wrap gap-2">
        <Badge variant="outline" className="tabular">
          {rows.length}
          {query && ` of ${processes.length}`} tracked
        </Badge>
        <div className="relative ml-auto w-full sm:w-auto">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2" />
          <Input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filter by name, user or PID"
            aria-label="Filter processes"
            className="w-full pl-8 sm:w-64"
          />
        </div>
      </CardHeader>

      <CardContent className="px-0 pb-2">
        {processes.length === 0 ? (
          <div className="px-5 py-16 text-center">
            <p className="text-sm">No process data</p>
            <p className="text-mute mt-1 text-xs">
              The daemon reports top processes with each sample. If this stays empty, check that
              sys-daemon is running.
            </p>
          </div>
        ) : rows.length === 0 ? (
          <div className="px-5 py-16 text-center">
            <p className="text-sm">Nothing matches “{query}”</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] text-sm">
              <thead>
                <tr className="text-mute border-line border-b font-mono text-2xs tracking-[0.15em] uppercase">
                  {COLUMNS.map((col) => {
                    const active = sortKey === col.key;
                    return (
                      <th
                        key={col.key}
                        scope="col"
                        aria-sort={active ? (descending ? 'descending' : 'ascending') : 'none'}
                        className={cn(
                          'px-5 py-2.5 font-medium',
                          col.numeric ? 'text-right' : 'text-left',
                          col.className,
                        )}
                      >
                        <button
                          type="button"
                          onClick={() => toggleSort(col.key)}
                          className={cn(
                            'hover:text-foreground inline-flex items-center gap-1 transition-colors',
                            active && 'text-foreground',
                          )}
                        >
                          {col.label}
                          {active &&
                            (descending ? (
                              <ArrowDown className="size-3" />
                            ) : (
                              <ArrowUp className="size-3" />
                            ))}
                        </button>
                      </th>
                    );
                  })}
                </tr>
              </thead>
              <tbody>
                {rows.map((proc) => (
                    <motion.tr
                      key={`${proc.pid}-${proc.name}`}
                      initial={{ opacity: 0 }}
                      animate={{ opacity: 1 }}
                      transition={{ duration: 0.15 }}
                      className="hover:bg-panel-strong/60 border-b border-transparent transition-colors last:border-0"
                    >
                      <td className="text-mute tabular px-5 py-2.5 text-right font-mono text-2xs">
                        {proc.pid}
                      </td>
                      <td className="max-w-[320px] truncate px-5 py-2.5 font-medium" title={proc.name}>
                        {proc.name}
                      </td>
                      <td className="text-mute px-5 py-2.5 font-mono text-2xs">{proc.user}</td>
                      <td className="px-5 py-2.5">
                        <div className="flex items-center justify-end gap-2.5">
                          <Meter
                            value={(proc.cpu / peakCPU) * 100}
                            tone={proc.cpu > 80 ? 'crit' : proc.cpu > 40 ? 'warn' : 'ok'}
                            className="w-20"
                            label={`${proc.name} CPU`}
                          />
                          <span
                            className={cn(
                              'tabular w-14 text-right font-mono text-2xs',
                              proc.cpu > 80 ? 'text-crit' : proc.cpu > 40 ? 'text-warn' : '',
                            )}
                          >
                            {proc.cpu.toFixed(1)}%
                          </span>
                        </div>
                      </td>
                      <td className="text-mute tabular px-5 py-2.5 text-right font-mono text-2xs">
                        {proc.memory} MB
                      </td>
                      <td className="px-5 py-2.5">
                        <Badge variant={stateVariant(proc.state)} className="px-2 py-0">
                          {proc.state}
                        </Badge>
                      </td>
                    </motion.tr>
                  ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
      </Card>
    </>
  );
};

export default Processes;
