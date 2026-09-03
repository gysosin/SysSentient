import React from 'react';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';
import { Badge } from './ui/badge';
import { Meter } from './ui/motion-primitives';
import SystemChart from './SystemChart';
import { cn } from '../lib/utils';
import { Filesystem, Process, SystemMetrics } from '../types';

/** Which tile was opened. */
export type MetricKey = 'cpu' | 'memory' | 'swap' | 'load' | 'diskio' | 'network' | 'temperature' | 'disk';

type Tone = 'ok' | 'warn' | 'crit' | 'neutral';

const TONE_TEXT: Record<Tone, string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  crit: 'text-crit',
  neutral: 'text-foreground',
};

const formatBytes = (bytes: number): string => {
  if (bytes >= 1024 ** 4) return `${(bytes / 1024 ** 4).toFixed(1)} TB`;
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(0)} MB`;
  return `${bytes} B`;
};

/** A labelled figure. Used for the summary strip at the top of every view. */
function Figure({ label, value, tone = 'neutral' }: { label: string; value: string; tone?: Tone }) {
  return (
    <div className="bg-panel-strong border-line rounded-lg border px-3 py-2.5">
      <div className="text-mute text-2xs tracking-[0.15em] uppercase">{label}</div>
      <div className={cn('font-display tabular mt-1 text-lg leading-none font-bold', TONE_TEXT[tone])}>
        {value}
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mt-5">
      <h3 className="text-mute border-line border-b pb-1.5 text-2xs font-semibold tracking-[0.2em] uppercase">
        {title}
      </h3>
      <div className="mt-3">{children}</div>
    </section>
  );
}

/**
 * Top consumers for the metric being examined.
 *
 * This is the "where" half of the question. A tile says the machine is at 90%;
 * only a ranked list says which process is responsible, which is the thing an
 * operator actually needs at 3am.
 */
function TopConsumers({
  processes,
  by,
}: {
  processes: Process[];
  by: 'cpu' | 'memory';
}) {
  const ranked = [...processes].sort((a, b) => b[by] - a[by]).slice(0, 8);
  if (ranked.length === 0) {
    return <p className="text-mute text-xs">No process data reported by the collector.</p>;
  }

  const peak = Math.max(...ranked.map((p) => p[by]), by === 'cpu' ? 100 : 1);

  return (
    <ul className="space-y-1.5">
      {ranked.map((p) => (
        <li key={p.pid} className="grid grid-cols-[1fr_auto] items-center gap-3">
          <div className="min-w-0">
            <div className="flex items-baseline gap-2">
              <span className="text-foreground truncate text-xs font-medium">{p.name}</span>
              <span className="text-melt shrink-0 font-mono text-2xs">{p.pid}</span>
              <span className="text-mute shrink-0 truncate text-2xs">{p.user}</span>
            </div>
            <Meter
              value={(p[by] / peak) * 100}
              tone={by === 'cpu' && p.cpu > 80 ? 'crit' : by === 'cpu' && p.cpu > 40 ? 'warn' : 'ok'}
              className="mt-1"
              label={`${p.name} ${by}`}
            />
          </div>
          <span className="tabular text-foreground shrink-0 font-mono text-xs">
            {by === 'cpu' ? `${p.cpu.toFixed(1)}%` : `${p.memory.toFixed(0)} MB`}
          </span>
        </li>
      ))}
    </ul>
  );
}

/** Per-core utilisation, so a pinned core is identifiable by number. */
function CoreBreakdown({ cores }: { cores: number[] }) {
  if (cores.length === 0) {
    return <p className="text-mute text-xs">No per-core data reported.</p>;
  }
  return (
    <div className="grid grid-cols-4 gap-2 sm:grid-cols-8">
      {cores.map((core, i) => {
        const crit = core > 85;
        const busy = core > 55;
        return (
          <div key={i} className="bg-panel-strong border-line rounded-md border px-2 py-1.5">
            <div className="text-melt font-mono text-2xs">C{i}</div>
            <Meter
              value={core}
              tone={crit ? 'crit' : busy ? 'warn' : 'ok'}
              className="mt-1"
              label={`core ${i}`}
            />
            <div
              className={cn(
                'tabular mt-1 font-mono text-2xs',
                crit ? 'text-crit' : busy ? 'text-warn' : 'text-mute',
              )}
            >
              {core.toFixed(0)}%
            </div>
          </div>
        );
      })}
    </div>
  );
}

/** Every mounted filesystem, with inode pressure alongside capacity. */
function FilesystemBreakdown({ filesystems }: { filesystems: Filesystem[] }) {
  if (filesystems.length === 0) {
    return <p className="text-mute text-xs">No filesystem capacity reported.</p>;
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[560px] text-left text-xs">
        <thead className="text-mute font-mono text-2xs tracking-[0.15em] uppercase">
          <tr className="border-line border-b">
            <th className="pb-2 font-medium">Mount</th>
            <th className="pb-2 font-medium">Device</th>
            <th className="pb-2 font-medium">Capacity</th>
            <th className="pb-2 font-medium">Used</th>
            <th className="pb-2 text-right font-medium">Inodes</th>
          </tr>
        </thead>
        <tbody className="font-mono">
          {filesystems.map((fs) => {
            const tone: Tone = fs.usedPercent >= 90 ? 'crit' : fs.usedPercent >= 80 ? 'warn' : 'ok';
            // Inodes fill independently of bytes: a volume full of tiny files
            // runs out of inodes while still showing free space, and the
            // resulting "no space left on device" is baffling without this.
            const inodeTone: Tone =
              fs.inodesUsedPercent >= 90 ? 'crit' : fs.inodesUsedPercent >= 80 ? 'warn' : 'ok';
            return (
              <tr key={fs.mountpoint} className="border-line hover:bg-panel-strong/60 border-b last:border-0">
                <td className="text-foreground py-2.5 font-medium">{fs.mountpoint}</td>
                <td className="text-mute py-2.5">{fs.device}</td>
                <td className="text-mute tabular py-2.5">
                  {formatBytes(fs.usedBytes)} / {formatBytes(fs.totalBytes)}
                </td>
                <td className="py-2.5">
                  <div className="flex items-center gap-2">
                    <Meter value={fs.usedPercent} tone={tone} className="w-16" label={fs.mountpoint} />
                    <span className={cn('tabular', TONE_TEXT[tone])}>{fs.usedPercent.toFixed(1)}%</span>
                  </div>
                </td>
                <td className={cn('tabular py-2.5 text-right', TONE_TEXT[inodeTone])}>
                  {fs.inodesUsedPercent.toFixed(1)}%
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

interface Props {
  metric: MetricKey | null;
  onClose: () => void;
  current: SystemMetrics;
  history: SystemMetrics[];
  processes: Process[];
}

/**
 * The analysis view behind every Overview tile.
 *
 * A tile answers "how much". This answers "of what, and where" — the history,
 * the breakdown, and the processes responsible. Without it, clicking a tile
 * that says 90% told you nothing you did not already know.
 */
export function MetricDrilldown({ metric, onClose, current, history, processes }: Props) {
  if (!metric) return null;

  const memPct = current.memoryTotal > 0 ? (current.memoryUsed / current.memoryTotal) * 100 : 0;
  const swapPct = current.swapTotal > 0 ? (current.swapUsed / current.swapTotal) * 100 : 0;
  const fullest = [...current.filesystems].sort((a, b) => b.usedPercent - a.usedPercent)[0];

  const views: Record<MetricKey, { title: string; description: string; body: React.ReactNode }> = {
    cpu: {
      title: 'Processor',
      description: 'Aggregate load, its distribution across cores, and the processes responsible.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure label="Total" value={`${current.cpuLoad.toFixed(1)}%`} tone={current.cpuLoad >= 90 ? 'crit' : current.cpuLoad >= 70 ? 'warn' : 'ok'} />
            <Figure label="Cores" value={String(current.cpuPerCore.length)} />
            <Figure label="Busiest core" value={current.cpuPerCore.length ? `${Math.max(...current.cpuPerCore).toFixed(0)}%` : 'n/a'} />
            <Figure label="Load 1m" value={current.loadAvg1.toFixed(2)} />
          </div>
          <Section title="Per-core distribution"><CoreBreakdown cores={current.cpuPerCore} /></Section>
          <Section title="Top consumers"><TopConsumers processes={processes} by="cpu" /></Section>
          <Section title="History">
            <SystemChart title="CPU usage" data={history} dataKey="cpuLoad" color="var(--chart-1)" unit="%" maxValue={100} />
          </Section>
        </>
      ),
    },
    memory: {
      title: 'Memory',
      description: 'What is allocated, what is reclaimable, and which processes hold it.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure label="Used" value={`${memPct.toFixed(1)}%`} tone={memPct >= 90 ? 'crit' : memPct >= 75 ? 'warn' : 'ok'} />
            <Figure label="Resident" value={`${(current.memoryUsed / 1024).toFixed(1)} GB`} />
            <Figure label="Total" value={`${(current.memoryTotal / 1024).toFixed(1)} GB`} />
            <Figure label="Swap" value={`${swapPct.toFixed(1)}%`} tone={swapPct >= 80 ? 'crit' : swapPct >= 40 ? 'warn' : 'ok'} />
          </div>
          <Section title="Top consumers"><TopConsumers processes={processes} by="memory" /></Section>
          <Section title="History">
            <SystemChart title="Memory used" data={history} dataKey="memoryUsed" color="var(--chart-2)" unit=" MB" />
          </Section>
        </>
      ),
    },
    swap: {
      title: 'Swap',
      description: 'Swap in use. Sustained swapping means memory pressure, not a swap problem.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure label="Used" value={`${swapPct.toFixed(1)}%`} tone={swapPct >= 80 ? 'crit' : swapPct >= 40 ? 'warn' : 'ok'} />
            <Figure label="Swap used" value={`${current.swapUsed.toFixed(0)} MB`} />
            <Figure label="Swap total" value={`${current.swapTotal.toFixed(0)} MB`} />
            <Figure label="Memory" value={`${memPct.toFixed(1)}%`} tone={memPct >= 90 ? 'crit' : memPct >= 75 ? 'warn' : 'ok'} />
          </div>
          <Section title="Memory holders">
            <p className="text-mute mb-3 text-xs">
              Swap fills because memory ran short, so the processes to look at are the memory
              consumers.
            </p>
            <TopConsumers processes={processes} by="memory" />
          </Section>
          <Section title="History">
            <SystemChart title="Swap used" data={history} dataKey="swapUsed" color="var(--chart-4)" unit=" MB" />
          </Section>
        </>
      ),
    },
    load: {
      title: 'Load average',
      description: 'Runnable and uninterruptible tasks, averaged. Compare against core count.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure label="1 minute" value={current.loadAvg1.toFixed(2)} tone={current.loadAvg1 >= 8 ? 'crit' : current.loadAvg1 >= 4 ? 'warn' : 'ok'} />
            <Figure label="5 minutes" value={current.loadAvg5.toFixed(2)} />
            <Figure label="15 minutes" value={current.loadAvg15.toFixed(2)} />
            <Figure label="Cores" value={String(current.cpuPerCore.length)} />
          </div>
          <Section title="Reading this">
            <p className="text-muted-foreground text-xs leading-relaxed">
              Load counts tasks waiting to run <em>and</em> tasks blocked on disk, so it can be high
              while the processor is idle. A sustained value above the core count
              {current.cpuPerCore.length > 0 && ` (${current.cpuPerCore.length} here)`} means work is
              queuing; 1m above 15m means it is getting worse.
            </p>
          </Section>
          <Section title="Top consumers"><TopConsumers processes={processes} by="cpu" /></Section>
          <Section title="History">
            <SystemChart title="Load average (1m)" data={history} dataKey="loadAvg1" color="var(--chart-3)" unit="" />
          </Section>
        </>
      ),
    },
    diskio: {
      title: 'Disk I/O',
      description: 'Throughput and operations per second across storage devices.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure label="IOPS" value={current.diskIOPS.toFixed(0)} />
            <Figure label="Read" value={`${current.diskRead.toFixed(1)} MB/s`} />
            <Figure label="Write" value={`${current.diskWrite.toFixed(1)} MB/s`} />
            <Figure label="Filesystems" value={String(current.filesystems.length)} />
          </div>
          <Section title="History">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <SystemChart title="Disk read" data={history} dataKey="diskRead" color="var(--chart-3)" unit=" MB/s" />
              <SystemChart title="Disk write" data={history} dataKey="diskWrite" color="var(--chart-4)" unit=" MB/s" />
            </div>
          </Section>
          <Section title="Mounted filesystems"><FilesystemBreakdown filesystems={current.filesystems} /></Section>
        </>
      ),
    },
    network: {
      title: 'Network',
      description: 'Ingress and egress throughput.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure label="Total" value={`${(current.networkRx + current.networkTx).toFixed(0)} KB/s`} />
            <Figure label="Received" value={`${current.networkRx.toFixed(0)} KB/s`} />
            <Figure label="Sent" value={`${current.networkTx.toFixed(0)} KB/s`} />
            <Figure label="Host" value={current.hostname || 'unknown'} />
          </div>
          <Section title="History">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <SystemChart title="Network RX" data={history} dataKey="networkRx" color="var(--chart-2)" unit=" KB/s" />
              <SystemChart title="Network TX" data={history} dataKey="networkTx" color="var(--chart-5)" unit=" KB/s" />
            </div>
          </Section>
        </>
      ),
    },
    temperature: {
      title: 'Temperature',
      description: 'Hottest reported sensor. Often the earliest sign of hardware trouble.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure
              label="Hottest"
              value={current.temperature > 0 ? `${current.temperature.toFixed(1)} °C` : 'n/a'}
              tone={current.temperature >= 90 ? 'crit' : current.temperature >= 80 ? 'warn' : 'ok'}
            />
            <Figure label="CPU load" value={`${current.cpuLoad.toFixed(1)}%`} />
            <Figure label="Cores" value={String(current.cpuPerCore.length)} />
            <Figure label="Load 1m" value={current.loadAvg1.toFixed(2)} />
          </div>
          {current.temperature === 0 && (
            <Section title="No sensor">
              <p className="text-muted-foreground text-xs leading-relaxed">
                No temperature sensor was reported. This is normal in virtual machines and many
                containers, where the host does not expose thermal data to the guest.
              </p>
            </Section>
          )}
          <Section title="History">
            <SystemChart title="Temperature" data={history} dataKey="temperature" color="var(--chart-4)" unit=" °C" />
          </Section>
        </>
      ),
    },
    disk: {
      title: 'Disk usage',
      description: 'Capacity and inode pressure for every mounted filesystem.',
      body: (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Figure
              label="Fullest"
              value={fullest ? `${fullest.usedPercent.toFixed(1)}%` : 'n/a'}
              tone={fullest && fullest.usedPercent >= 90 ? 'crit' : fullest && fullest.usedPercent >= 80 ? 'warn' : 'ok'}
            />
            <Figure label="Mount" value={fullest ? fullest.mountpoint : 'n/a'} />
            <Figure label="Free" value={fullest ? formatBytes(fullest.freeBytes) : 'n/a'} />
            <Figure label="Mounts" value={String(current.filesystems.length)} />
          </div>
          <Section title="All filesystems"><FilesystemBreakdown filesystems={current.filesystems} /></Section>
        </>
      ),
    },
  };

  const view = views[metric];

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[88vh] max-w-4xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-3">
            {view.title}
            <Badge variant="outline">live</Badge>
          </DialogTitle>
          <DialogDescription>{view.description}</DialogDescription>
        </DialogHeader>
        {view.body}
      </DialogContent>
    </Dialog>
  );
}
