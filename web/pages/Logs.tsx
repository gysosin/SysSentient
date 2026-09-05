import React, { useMemo, useState } from 'react';
import { motion } from 'motion/react';
import { Search, Terminal } from 'lucide-react';

import { LogEntry } from '../types';
import { useDashboard, useFeed } from '../hooks/useDashboardData';
import { Card, CardContent } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { ScreenHeading } from '../components/ui/section-heading';
import { cn } from '../lib/utils';

const LEVELS: LogEntry['level'][] = ['ERROR', 'WARN', 'INFO'];

/**
 * Clock time for a log line.
 *
 * Entries arrive as full ISO strings. Rendered raw they truncate to
 * "2026-09-03T0…" in any column narrow enough to leave room for the message,
 * which is worse than useless — it looks like a timestamp while telling you
 * nothing. During an incident the only part anyone reads is the time of day,
 * and the date is already established by the fact that these are recent logs.
 */
const clockOf = (timestamp: string): string => {
  const parsed = new Date(timestamp);
  if (!Number.isNaN(parsed.getTime())) {
    return parsed.toLocaleTimeString('en-GB', { hour12: false });
  }
  // Not ISO: fall back to the first time-looking run of digits, then to the
  // raw value rather than showing nothing.
  const match = timestamp.match(/\d{1,2}:\d{2}:\d{2}/);
  return match ? match[0] : timestamp;
};

const levelTone = (level: LogEntry['level']) =>
  level === 'ERROR' ? 'text-crit' : level === 'WARN' ? 'text-warn' : 'text-mute';

/**
 * The streaming/halted pill.
 *
 * Its own component because it reads the feed, which republishes every second.
 * Consuming that from the page would redraw the whole log list on every tick.
 */
const StreamingPill: React.FC = () => {
  const feed = useFeed();
  const streaming = feed.level === 'live' || feed.level === 'polling';
  return (
    <span className={cn('flex items-center gap-1.5 font-medium', streaming ? 'text-ok' : 'text-mute')}>
      <span
        className={cn('size-1.5 rounded-full', streaming ? 'live-pulse bg-ok' : 'bg-melt')}
        aria-hidden="true"
      />
      {streaming ? 'streaming' : 'halted'}
    </span>
  );
};

const Logs: React.FC = () => {
  const { logs, current } = useDashboard();
  const [query, setQuery] = useState('');
  const [enabled, setEnabled] = useState<Set<LogEntry['level']>>(new Set(LEVELS));

  const toggleLevel = (level: LogEntry['level']) => {
    setEnabled((prev) => {
      const next = new Set(prev);
      if (next.has(level)) next.delete(level);
      else next.add(level);
      return next;
    });
  };

  const counts = useMemo(() => {
    const acc: Record<string, number> = { ERROR: 0, WARN: 0, INFO: 0 };
    for (const log of logs) acc[log.level] = (acc[log.level] ?? 0) + 1;
    return acc;
  }, [logs]);

  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return logs.filter(
      (log) =>
        enabled.has(log.level) &&
        (!needle ||
          log.message.toLowerCase().includes(needle) ||
          log.facility.toLowerCase().includes(needle)),
    );
  }, [logs, query, enabled]);


  return (
    <>
      <ScreenHeading
        eyebrow="Signal stream"
        title="Logs"
        description="Recent system events, with severity carried in the line itself so a scan finds the problem before any reading starts."
      />

      <Card>
        <CardContent className="p-5">
          {/* Controls. Level filters are toggles rather than a single-select
              because during an incident you usually want ERROR and WARN
              together and INFO gone, which a radio group cannot express. */}
          <div className="mb-5 flex flex-wrap items-center gap-2">
            <div className="border-line flex overflow-hidden rounded-md border">
              {LEVELS.map((level) => {
                const on = enabled.has(level);
                return (
                  <button
                    key={level}
                    type="button"
                    onClick={() => toggleLevel(level)}
                    aria-pressed={on}
                    className={cn(
                      'border-line px-3 py-2 font-mono text-2xs font-semibold transition-colors not-last:border-r',
                      on
                        ? level === 'ERROR'
                          ? 'bg-crit-soft text-crit'
                          : level === 'WARN'
                            ? 'bg-warn-soft text-warn'
                            : 'bg-panel-strong text-foreground'
                        : // Struck through, not merely dimmed: "off" has to be
                          // unambiguous, and low opacity alone reads as
                          // disabled rather than as excluded.
                          'text-melt line-through',
                    )}
                  >
                    {level} {counts[level] ?? 0}
                  </button>
                );
              })}
            </div>

            <div className="relative ml-auto">
              <Search className="text-melt pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2" />
              <Input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search messages"
                aria-label="Search log messages"
                className="w-56 pl-8 font-mono"
              />
            </div>
          </div>

          {/* A terminal, not a table. Logs are read as a stream of lines, and
              the recessed dark ground separates machine output from the
              console's own chrome. */}
          <div className="border-line bg-background overflow-hidden rounded-lg border">
            <div className="border-line bg-panel-strong text-mute flex items-center justify-between gap-3 border-b px-4 py-2 font-mono text-2xs">
              <span className="text-foreground truncate font-semibold">
                {current.hostname || 'host'} — journald stream
              </span>
              <span className="flex shrink-0 items-center gap-3">
                <span className="tabular">
                  {rows.length}
                  {rows.length !== logs.length && ` of ${logs.length}`} events
                </span>
                <StreamingPill />
              </span>
            </div>

            {logs.length === 0 ? (
              <div className="px-4 py-16 text-center">
                <Terminal className="text-melt mx-auto size-5" aria-hidden="true" />
                <p className="mt-3 text-sm">No log entries</p>
                <p className="text-mute mx-auto mt-1 max-w-lg text-xs">
                  The daemon reads journald and dmesg. Under the shipped systemd unit dmesg is
                  blocked by ProtectKernelLogs — journald is the working source.
                </p>
              </div>
            ) : rows.length === 0 ? (
              <div className="px-4 py-16 text-center">
                <Terminal className="text-melt mx-auto size-5" aria-hidden="true" />
                <p className="mt-3 text-sm">No entries match the current filters</p>
                <p className="text-mute mt-1 text-xs">
                  Clear the search or re-enable a severity above.
                </p>
              </div>
            ) : (
              <ul className="max-h-[65vh] overflow-y-auto p-3 font-mono text-2xs">
                {rows.map((log, idx) => (
                  <motion.li
                    key={`${log.timestamp}-${idx}`}
                    initial={{ opacity: 0, x: -4 }}
                    animate={{ opacity: 1, x: 0 }}
                    transition={{ duration: 0.18, delay: Math.min(idx * 0.01, 0.25) }}
                    className="hover:bg-panel-strong/60 border-line/40 grid grid-cols-[72px_58px_120px_minmax(0,1fr)] gap-3 border-b px-2 py-1 leading-6 transition-colors last:border-0"
                  >
                    {/* A log line without a time is barely a log line — the
                        previous viewer dropped it entirely, so you could not
                        tell whether a spike was seconds or hours ago. */}
                    <span className="text-melt tabular truncate select-none" title={log.timestamp}>
                      {clockOf(log.timestamp)}
                    </span>
                    <span className={cn('font-semibold uppercase select-none', levelTone(log.level))}>
                      {log.level}
                    </span>
                    <span className="text-foreground/70 truncate" title={log.facility}>
                      {log.facility}
                    </span>
                    <span className="text-muted-foreground break-words">{log.message}</span>
                  </motion.li>
                ))}
              </ul>
            )}
          </div>
        </CardContent>
      </Card>
    </>
  );
};

export default Logs;
