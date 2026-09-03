import React from 'react';
import { Link } from 'react-router-dom';
import { motion } from 'motion/react';
import {
  BrainCircuit,
  ChevronRight,
  Cpu,
  Gauge,
  HardDrive,
  MemoryStick,
  Network,
  Thermometer,
  Timer,
  Waves,
} from 'lucide-react';

import { useDashboard } from '../hooks/useDashboardData';
import SystemChart from '../components/SystemChart';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/card';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Skeleton } from '../components/ui/skeleton';
import { HairlineGrid, SectionHeading } from '../components/ui/section-heading';
import {
  AnimatedNumber,
  Meter,
  Sparkline,
  Stagger,
  StaggerItem,
} from '../components/ui/motion-primitives';
import { cn } from '../lib/utils';
import { formatDuration } from '../lib/utils';
import { SystemMetrics } from '../types';

type Tone = 'ok' | 'warn' | 'crit' | 'neutral';

// Never returns "neutral": a measured value always has a severity.
const toneFor = (value: number, warn: number, crit: number): Exclude<Tone, 'neutral'> =>
  value >= crit ? 'crit' : value >= warn ? 'warn' : 'ok';

type Severity = Exclude<Tone, 'neutral'>;

/**
 * Severity with hysteresis: quick to escalate, slow to stand down.
 *
 * A raw threshold comparison is unusable at a two-second refresh. A machine
 * sitting at 89-91C against a 90C limit does not have a flickering problem,
 * but a naive tile flips between "warning" and "critical" every sample, and
 * the page headline - 72px of display type - rewrites itself continuously.
 * That is the exact failure the brief calls out: an animation that is charming
 * once is intolerable at 1,800 repetitions an hour.
 *
 * So the thresholds widen once crossed. Escalation is immediate, because a
 * real breach must never be delayed; recovery has to clear the threshold by a
 * margin before the interface relaxes. Same reasoning as the `for:` duration
 * on an alert rule, applied to colour instead of paging.
 */
function useStickyTone(value: number, warn: number, crit: number): Severity {
  const [tone, setTone] = React.useState<Severity>(() => toneFor(value, warn, crit));

  React.useEffect(() => {
    if (!Number.isFinite(value)) return;
    // A quarter of the band between the thresholds, so the margin scales with
    // the metric: ~5 points for CPU percentages, 1.0 for load average.
    const release = Math.max(0.5, (crit - warn) * 0.25);
    setTone((prev) => {
      const critAt = prev === 'crit' ? crit - release : crit;
      const warnAt = prev === 'ok' ? warn : warn - release;
      if (value >= critAt) return 'crit';
      if (value >= warnAt) return 'warn';
      return 'ok';
    });
  }, [value, warn, crit]);

  return tone;
}

const percent = (used: number, total: number) =>
  total > 0 ? Math.min(100, (used / total) * 100) : 0;

const formatBytes = (bytes: number): string => {
  if (bytes >= 1024 ** 4) return `${(bytes / 1024 ** 4).toFixed(1)} TB`;
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(0)} MB`;
  return `${bytes} B`;
};

const VALUE_TONE: Record<Tone, string> = {
  ok: 'text-foreground',
  warn: 'text-warn',
  crit: 'text-crit',
  neutral: 'text-foreground',
};

const RING_TONE: Record<Tone, string> = {
  ok: 'border-line',
  warn: 'border-warn/40',
  crit: 'border-crit/50',
  neutral: 'border-line',
};

const FILL_TONE: Record<Tone, string> = {
  ok: 'bg-ok',
  warn: 'bg-warn',
  crit: 'bg-crit',
  neutral: 'bg-chart-2',
};

interface StatProps {
  label: string;
  icon: React.ElementType;
  value: number;
  decimals?: number;
  suffix?: string;
  display?: string;
  sub: string;
  tone?: Tone;
  series?: number[];
  loading?: boolean;
  /** 0-100 fill for the tile's capacity rule, when the metric has a ceiling. */
  fill?: number;
  /** Categorical colour for metrics that have no severity — throughput has no
   *  "bad" value, so tinting it by threshold would invent a judgement. */
  seriesColor?: string;
}

function Stat({
  label,
  icon: Icon,
  value,
  decimals = 0,
  suffix = '',
  display,
  sub,
  tone = 'neutral',
  series,
  loading,
  fill,
  seriesColor,
}: StatProps) {
  const strokeFor: Record<Tone, string> = {
    ok: 'var(--ok)',
    warn: 'var(--warn)',
    crit: 'var(--crit)',
    // Not --brand: coral is identity and liveness, so a plain measurement must
    // not borrow it and read as "the thing this screen is about".
    neutral: 'var(--chart-2)',
  };

  return (
    <Card className={cn('h-full transition-colors', RING_TONE[tone])}>
      <CardContent className="px-3.5 pt-3.5 pb-4">
        <div className="flex items-center gap-1.5">
          <Icon className="text-melt size-3.5 shrink-0" aria-hidden="true" />
          <span className="text-mute truncate text-[10px] font-medium tracking-[0.2em] uppercase">
            {label}
          </span>
        </div>

        {loading ? (
          <Skeleton className="mt-3 h-7 w-20" />
        ) : (
          <p
            className={cn(
              'font-display mt-2 text-[26px] leading-none font-bold',
              VALUE_TONE[tone],
            )}
          >
            {display ?? <AnimatedNumber value={value} decimals={decimals} suffix={suffix} />}
          </p>
        )}

        {/* A capacity rule directly under the number, so "68%" is visibly a
            proportion of something rather than a bare figure. Metrics with no
            ceiling (throughput) get no rule rather than a meaningless one. */}
        {fill !== undefined && !loading && (
          <div className="bg-panel-strong mt-3 h-1.5 overflow-hidden rounded-full">
            <motion.div
              className={cn('h-full rounded-full', FILL_TONE[tone])}
              initial={false}
              animate={{ width: `${Math.min(100, Math.max(0, fill))}%` }}
              transition={{ type: 'spring', stiffness: 180, damping: 26 }}
            />
          </div>
        )}

        {/* A <p> cannot legally contain the Skeleton's <div>, and React warns
            that it will break hydration. Use a block-level wrapper. */}
        <div className="text-mute tabular mt-2 truncate text-[11px]" title={sub}>
          {loading ? <Skeleton className="h-3 w-32" /> : sub}
        </div>

        {series && series.length > 1 && (
          <Sparkline
            data={series}
            stroke={tone === 'neutral' && seriesColor ? seriesColor : strokeFor[tone]}
            className="mt-2.5 h-7"
          />
        )}

        {/* Severity as a word as well as a colour. A tile that is fine says so
            — silence is ambiguous, and colour alone fails for anyone who cannot
            distinguish these hues or is looking from across a room. */}
        {tone !== 'neutral' && !loading && (
          <Badge variant={tone} className="mt-2.5">
            {tone === 'crit' ? 'critical' : tone === 'warn' ? 'warning' : 'normal'}
          </Badge>
        )}
      </CardContent>
    </Card>
  );
}

/** Per-core utilisation. */
function CoreGrid({ cores }: { cores: number[] }) {
  if (cores.length === 0) {
    return <p className="text-mute text-xs">No per-core data reported.</p>;
  }

  return (
    // A named cell per core rather than an anonymous bar. During an incident
    // the question is "which core is pinned", and a strip of unlabelled slivers
    // cannot answer it. Wraps rather than squeezing, so 64 cores stay legible.
    <div
      className="grid grid-cols-4 gap-2 sm:grid-cols-6 lg:grid-cols-8"
      role="img"
      aria-label={`${cores.length} CPU cores, highest ${Math.max(...cores).toFixed(0)}%`}
    >
      {cores.map((core, i) => {
        const critical = core > 85;
        const busy = core > 55;
        const fill = critical ? 'bg-crit' : busy ? 'bg-warn' : 'bg-ok';

        return (
          <div
            key={i}
            className="bg-panel-strong border-line flex flex-col gap-1.5 rounded-md border px-2 py-1.5"
          >
            <span className="text-melt font-mono text-[9px] tracking-wide">C{i}</span>

            {/* Explicit track: without a visible empty region the fill has
                nothing to be a proportion of. */}
            <div className="bg-background/60 relative h-1.5 w-full overflow-hidden rounded-full">
              <motion.div
                className={cn('absolute inset-y-0 left-0 rounded-full', fill)}
                initial={false}
                animate={{ width: `${Math.min(100, Math.max(2, core))}%` }}
                transition={{ type: 'spring', stiffness: 180, damping: 24 }}
              />
            </div>

            <span
              className={cn(
                'tabular font-mono text-[10px] font-medium',
                critical ? 'text-crit' : busy ? 'text-warn' : 'text-mute',
              )}
            >
              {core.toFixed(0)}%
            </span>
          </div>
        );
      })}
    </div>
  );
}

/** One cell of the hairline KPI strip. */
function Kpi({
  label,
  value,
  detail,
  tone = 'text-foreground',
  fill,
}: {
  label: string;
  value: string;
  detail: string;
  tone?: string;
  fill?: number;
}) {
  return (
    <div className="bg-panel p-4 sm:p-5">
      <div className="text-mute text-[10px] tracking-[0.2em] uppercase">{label}</div>
      <div className="mt-2 flex items-end gap-2">
        <span className={cn('font-display tabular text-3xl leading-none font-bold', tone)}>
          {value}
        </span>
      </div>
      {/* Only drawn where a real proportion exists. A bar with no denominator
          is decoration pretending to be data. */}
      {fill !== undefined && (
        <div className="bg-panel-strong mt-3 h-1.5 overflow-hidden rounded-full">
          <div
            className={cn('h-full rounded-full', tone.replace('text-', 'bg-'))}
            style={{ width: `${Math.min(100, Math.max(0, fill))}%` }}
          />
        </div>
      )}
      <div className="text-mute mt-2 text-[11px]">{detail}</div>
    </div>
  );
}

const seriesOf = (history: SystemMetrics[], key: keyof SystemMetrics): number[] =>
  history.slice(-32).map((m) => Number(m[key]) || 0);

const Overview: React.FC = () => {
  const { current, metricsHistory, hasData, feed, processes, logs, hosts, firingAlerts, ai } =
    useDashboard();
  const loading = !hasData;
  const stale = feed.level === 'stale' || feed.level === 'down';

  const memPct = percent(current.memoryUsed, current.memoryTotal);
  const swapPct = percent(current.swapUsed, current.swapTotal);
  // Stabilised severities. The tiles, the capacity bars and the headline all
  // read from these, so the big statement at the top can never contradict the
  // grid underneath it.
  const cpuTone = useStickyTone(current.cpuLoad, 70, 90);
  const memTone = useStickyTone(memPct, 75, 90);
  const swapTone = useStickyTone(swapPct, 40, 80);
  const tempSeverity = useStickyTone(current.temperature, 80, 90);
  const loadTone = useStickyTone(current.loadAvg1, 4, 8);
  const tempTone: Tone = current.temperature > 0 ? tempSeverity : 'neutral';

  // The fullest filesystem drives the headline, because a full disk is the
  // single most common cause of an outage and the one the eight tiles are most
  // likely to bury.
  const fullestDisk = [...current.filesystems].sort(
    (a, b) => b.usedPercent - a.usedPercent,
  )[0];
  const diskSeverity = useStickyTone(fullestDisk ? fullestDisk.usedPercent : 0, 80, 90);
  const diskTone: Tone = fullestDisk ? diskSeverity : 'neutral';

  // Whole-machine verdict. Deliberately computed from the same thresholds the
  // tiles use, so the headline can never disagree with the grid underneath it.
  const concerns = [
    { tone: cpuTone, subject: 'processor load' },
    { tone: memTone, subject: 'memory' },
    { tone: swapTone, subject: 'swap' },
    { tone: tempTone, subject: 'temperature' },
    {
      tone: diskTone,
      subject: fullestDisk ? `disk on ${fullestDisk.mountpoint}` : 'disk usage',
    },
  ].filter((c) => c.tone === 'warn' || c.tone === 'crit');

  const critical = concerns.filter((c) => c.tone === 'crit');
  const verdictTone: Tone = critical.length > 0 ? 'crit' : concerns.length > 0 ? 'warn' : 'ok';

  const headline =
    verdictTone === 'crit'
      ? { lead: 'Attention needed on', subject: `${critical[0].subject}.` }
      : verdictTone === 'warn'
        ? { lead: 'Watch', subject: `${concerns[0].subject}.` }
        : { lead: 'All signals', subject: 'nominal.' };

  const verdictProse = loading
    ? 'Waiting for the first sample from the collector.'
    : stale
      ? `The feed is ${feed.label.toLowerCase()}. Every value on this screen is history, not current state.`
      : concerns.length === 0
        ? `Realtime health for ${current.hostname || 'this host'}. The machine is online, the feed is current, and nothing is above threshold.`
        : `Realtime health for ${current.hostname || 'this host'}. ${concerns.length} of 5 tracked resources ${concerns.length === 1 ? 'is' : 'are'} above threshold; the rest are nominal.`;

  // gopsutil already counts cache and buffers outside `used`, so the three
  // segments sum to the total without double-counting. Hosts that report no
  // breakdown (some containers, some BSDs) fall back to a single bar rather
  // than rendering a fabricated split.
  const cacheMB = current.memoryCached + current.memoryBuffers;
  const hasMemoryComposition = cacheMB > 0;
  const appMB = current.memoryUsed;
  const appPct = percent(appMB, current.memoryTotal);
  const cachePct = percent(cacheMB, current.memoryTotal);

  const errorLogs = logs.filter((l) => l.level === 'ERROR').length;
  const warnLogs = logs.filter((l) => l.level === 'WARN').length;

  // Top consumers, sorted here rather than trusting collector order.
  const triage = [...processes].sort((a, b) => b.cpu - a.cpu).slice(0, 4);

  return (
    <div className={cn('space-y-6', stale && !loading && 'opacity-60 transition-opacity')}>
      {/* THE GLANCE. Answers "is anything wrong?" before any reading happens —
          from the size and colour of one phrase, at wall-display distance. */}
      <section className="border-line bg-panel relative overflow-hidden rounded-xl border px-5 py-7 sm:px-8 md:py-10">
        <div className="scanline" aria-hidden="true" />
        <div className="relative max-w-3xl">
          <div className="text-mute flex flex-wrap items-center gap-3 text-[10px] font-semibold tracking-[0.22em] uppercase">
            <span
              className={cn(
                'size-2 rounded-full',
                feed.level === 'live' ? 'live-pulse bg-brand' : stale ? 'bg-crit' : 'bg-warn',
              )}
              aria-hidden="true"
            />
            {feed.level === 'live' ? 'Live telemetry' : `${feed.label} telemetry`}
            <span className="text-melt">/</span>
            <span className="text-foreground font-mono tracking-normal">
              {current.hostname || 'unknown host'}
            </span>
            <span className="text-melt">/</span>
            <span>up {formatDuration(current.uptimeSeconds)}</span>
          </div>

          {/* Three size steps, not two. The reference headline was fixed copy;
              ours is generated and varies in length, so a single jump from
              48px to 72px let a long verdict eat 44% of a tablet viewport and
              pushed the eight tiles - the actual glance content - below the
              fold. */}
          <h1 className="text-foreground font-display mt-5 max-w-3xl text-4xl leading-[0.95] font-bold tracking-tight uppercase sm:text-5xl sm:leading-[0.9] lg:text-7xl">
            {headline.lead}
            <br />
            <span
              className={cn(
                verdictTone === 'crit'
                  ? 'text-crit'
                  : verdictTone === 'warn'
                    ? 'text-warn'
                    : 'text-brand',
              )}
            >
              {headline.subject}
            </span>
          </h1>

          <p className="text-muted-foreground mt-5 max-w-xl text-sm leading-relaxed">
            {verdictProse}
          </p>

          <div className="mt-7 flex flex-wrap items-center gap-3">
            {/* Amber, and only here: this is the machine being asked for an
                opinion, which is a different kind of action from navigating. */}
            <Button variant="accent" onClick={() => void ai.run()} disabled={ai.loading}>
              <BrainCircuit aria-hidden="true" />
              {ai.loading ? 'Analysing…' : 'Run AI diagnosis'}
            </Button>
            <Button variant="outline" asChild>
              <Link to={firingAlerts > 0 ? '/alerts' : '/processes'}>
                {firingAlerts > 0
                  ? `Review ${firingAlerts} firing alert${firingAlerts === 1 ? '' : 's'}`
                  : 'Inspect processes'}
                <ChevronRight aria-hidden="true" />
              </Link>
            </Button>
            <span className="text-mute text-[11px]">
              {loading ? 'awaiting first sample' : `last sample ${feed.detail}`}
            </span>
          </div>
        </div>
      </section>

      {/* Fleet-level counts. Separate from the eight tiles because these
          describe the installation, not the machine. */}
      <HairlineGrid className="grid-cols-2 md:grid-cols-4">
        <Kpi
          label="Fleet"
          value={String(Math.max(hosts.length, 1))}
          detail={hosts.length > 1 ? `${hosts.length} agents reporting` : 'single-node install'}
          tone="text-ok"
        />
        <Kpi
          label="Processes"
          value={loading ? '—' : String(processes.length)}
          detail={triage.length > 0 ? `top consumer ${triage[0].name}` : 'no process data'}
          tone="text-brand"
        />
        <Kpi
          label="Log events"
          value={loading ? '—' : String(logs.length)}
          detail={`${errorLogs} error · ${warnLogs} warning`}
          tone={errorLogs > 0 ? 'text-crit' : 'text-foreground'}
          fill={logs.length > 0 ? ((errorLogs + warnLogs) / logs.length) * 100 : undefined}
        />
        <Kpi
          label="Firing alerts"
          value={String(firingAlerts)}
          detail={firingAlerts === 0 ? 'nothing firing' : 'needs acknowledgement'}
          tone={firingAlerts > 0 ? 'text-crit' : 'text-ok'}
        />
      </HairlineGrid>

      {/* Eight tiles on one line at desktop width: whole-machine health
          readable in a single horizontal sweep rather than two rows the eye
          has to reconcile. */}
      <Stagger className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8">
        <StaggerItem>
          <Stat
            label="CPU"
            icon={Cpu}
            value={current.cpuLoad}
            decimals={1}
            suffix="%"
            sub={`${current.cpuPerCore.length} cores`}
            tone={cpuTone}
            fill={current.cpuLoad}
            series={seriesOf(metricsHistory, 'cpuLoad')}
            loading={loading}
          />
        </StaggerItem>
        <StaggerItem>
          <Stat
            label="Memory"
            icon={MemoryStick}
            value={memPct}
            suffix="%"
            sub={`${(current.memoryUsed / 1024).toFixed(1)} / ${(current.memoryTotal / 1024).toFixed(1)} GB`}
            tone={memTone}
            fill={memPct}
            series={seriesOf(metricsHistory, 'memoryUsed')}
            loading={loading}
          />
        </StaggerItem>
        <StaggerItem>
          <Stat
            label="Swap"
            icon={Waves}
            value={swapPct}
            suffix="%"
            sub={`${current.swapUsed.toFixed(0)} / ${current.swapTotal.toFixed(0)} MB`}
            tone={swapTone}
            fill={swapPct}
            series={seriesOf(metricsHistory, 'swapUsed')}
            loading={loading}
          />
        </StaggerItem>
        <StaggerItem>
          <Stat
            label="Load average"
            icon={Gauge}
            value={current.loadAvg1}
            decimals={2}
            sub={`5m ${current.loadAvg5.toFixed(2)} · 15m ${current.loadAvg15.toFixed(2)}`}
            tone={loadTone}
            series={seriesOf(metricsHistory, 'loadAvg1')}
            loading={loading}
          />
        </StaggerItem>

        <StaggerItem>
          <Stat
            label="Disk I/O"
            icon={HardDrive}
            value={current.diskIOPS}
            suffix=" iops"
            sub={`read ${current.diskRead.toFixed(1)} · write ${current.diskWrite.toFixed(1)} MB/s`}
            series={seriesOf(metricsHistory, 'diskWrite')}
            seriesColor="var(--chart-3)"
            loading={loading}
          />
        </StaggerItem>
        <StaggerItem>
          <Stat
            label="Network"
            icon={Network}
            value={current.networkRx + current.networkTx}
            suffix=" KB/s"
            sub={`rx ${current.networkRx.toFixed(0)} · tx ${current.networkTx.toFixed(0)} KB/s`}
            series={seriesOf(metricsHistory, 'networkRx')}
            loading={loading}
          />
        </StaggerItem>
        <StaggerItem>
          <Stat
            label="Temperature"
            icon={Thermometer}
            value={current.temperature}
            display={current.temperature > 0 ? undefined : 'n/a'}
            suffix="°C"
            sub={current.temperature > 0 ? 'hottest sensor' : 'no sensor reported'}
            tone={tempTone}
            series={seriesOf(metricsHistory, 'temperature')}
            loading={loading}
          />
        </StaggerItem>
        <StaggerItem>
          <Stat
            label="Disk usage"
            icon={Timer}
            value={fullestDisk ? fullestDisk.usedPercent : 0}
            suffix="%"
            display={fullestDisk ? undefined : 'n/a'}
            sub={
              fullestDisk
                ? `${fullestDisk.mountpoint} · ${formatBytes(fullestDisk.freeBytes)} free`
                : 'no filesystems reported'
            }
            tone={diskTone}
            fill={fullestDisk ? fullestDisk.usedPercent : undefined}
            loading={loading}
          />
        </StaggerItem>
      </Stagger>

      {/* WHAT IS CONSUMING THE MACHINE, and what the machine thinks about it.
          Side by side because during an incident those two questions are asked
          together. */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-5">
          <CardContent className="p-5">
            <SectionHeading
              eyebrow="Current attention"
              title="Process triage"
              action={
                <span className="text-mute tabular font-mono text-[10px]">
                  {processes.length} running
                </span>
              }
            />
            <div className="mt-5 space-y-2">
              {loading && (
                <>
                  <Skeleton className="h-[62px] w-full" />
                  <Skeleton className="h-[62px] w-full" />
                  <Skeleton className="h-[62px] w-full" />
                </>
              )}
              {!loading && triage.length === 0 && (
                <p className="text-mute py-6 text-center text-xs">
                  No process data reported by the collector.
                </p>
              )}
              {triage.map((p) => {
                const crit = p.cpu >= 80 || p.state === 'Zombie';
                return (
                  <div
                    key={p.pid}
                    className={cn(
                      'flex items-center gap-3 rounded-lg border p-3',
                      crit ? 'border-crit/40 bg-crit-soft' : 'border-line bg-panel-strong',
                    )}
                  >
                    {/* A square state chip, not just coloured text: severity has
                        to survive peripheral vision and colour-blindness. */}
                    <span
                      className={cn(
                        'grid size-9 shrink-0 place-items-center rounded-md font-mono text-[10px] font-semibold',
                        crit ? 'bg-crit/15 text-crit' : 'bg-ok/10 text-ok',
                      )}
                    >
                      {crit ? 'CRIT' : 'OK'}
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="text-foreground truncate text-[13px] font-medium">
                        {p.name} <span className="text-mute font-mono">· {p.pid}</span>
                      </p>
                      <p className="text-mute truncate text-[11px]">
                        {p.user} · {p.memory.toFixed(0)} MB resident · {p.state.toLowerCase()}
                      </p>
                    </div>
                    <div className="shrink-0 text-right">
                      <p
                        className={cn(
                          'tabular font-mono text-[13px]',
                          crit ? 'text-crit' : 'text-foreground',
                        )}
                      >
                        {p.cpu.toFixed(1)}%
                      </p>
                      <p className="text-melt text-[10px]">cpu</p>
                    </div>
                  </div>
                );
              })}
            </div>
            <Button variant="outline" className="mt-5 w-full" asChild>
              <Link to="/processes">
                View all processes <ChevronRight aria-hidden="true" />
              </Link>
            </Button>
          </CardContent>
        </Card>

        <Card className="relative overflow-hidden lg:col-span-7">
          {/* Amber hairline. The one piece of chrome that marks a panel as
              model-generated rather than measured. */}
          <div className="bg-accent absolute inset-x-0 top-0 h-px" aria-hidden="true" />
          <CardContent className="p-5">
            <SectionHeading
              eyebrow="Accountable assistance"
              title="AI diagnosis"
              action={
                <span className="text-mute font-mono text-[10px]">
                  {ai.result ? ai.result.status.toLowerCase() : 'not run'}
                </span>
              }
            />

            <div className="border-line bg-panel-strong mt-5 rounded-lg border p-4">
              <div className="text-accent flex items-center gap-2 text-[10px] font-semibold tracking-[0.2em] uppercase">
                <BrainCircuit className="size-3.5" aria-hidden="true" />
                {ai.error
                  ? 'Analysis failed'
                  : ai.loading
                    ? 'Evaluating telemetry'
                    : ai.result
                      ? `Verdict · ${ai.result.status}`
                      : 'No analysis yet'}
              </div>

              {ai.loading ? (
                <div className="mt-3 space-y-2">
                  <Skeleton className="h-3 w-full" />
                  <Skeleton className="h-3 w-4/5" />
                </div>
              ) : (
                <p className="text-foreground/85 mt-3 text-[13px] leading-relaxed">
                  {ai.error
                    ? ai.error
                    : (ai.result?.summary ??
                      'Nothing has been sent for analysis yet. Running a diagnosis submits a redacted summary of current state and returns an explanation with suggested commands.')}
                </p>
              )}

              {ai.result && ai.result.recommendedActions.length > 0 && (
                <div className="mt-4 flex flex-wrap gap-2">
                  {ai.result.recommendedActions.slice(0, 3).map((action) => (
                    <span
                      key={action.id}
                      className={cn(
                        'rounded-full border px-2.5 py-1 font-mono text-[10px]',
                        action.isSafe
                          ? 'border-line bg-panel text-mute'
                          : 'border-crit/40 bg-crit-soft text-crit',
                      )}
                    >
                      {action.isSafe ? action.command.split(' ')[0] : `review: ${action.command.split(' ')[0]}`}
                    </span>
                  ))}
                </div>
              )}
            </div>

            <div className="mt-4 flex flex-wrap gap-2">
              <Button variant="accent" onClick={() => void ai.run()} disabled={ai.loading}>
                <BrainCircuit aria-hidden="true" />
                {ai.result ? 'Refresh analysis' : 'Analyze current state'}
              </Button>
              <Button variant="outline" asChild>
                <Link to="/insights">
                  Open full insight <ChevronRight aria-hidden="true" />
                </Link>
              </Button>
            </div>

            {/* Non-negotiable. The model suggests shell commands, and the design
                has to make that read as advice to evaluate, never an
                instruction to obey. */}
            <p className="text-mute mt-4 text-[11px]">
              AI suggestions are advisory. Commands are shown for review and are never executed
              automatically.
            </p>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-5">
        <Card className="lg:col-span-2">
          <CardContent className="p-5">
            <SectionHeading
              eyebrow="Composition"
              title="Memory allocation"
              action={
                <span className="text-mute tabular font-mono text-[10px]">
                  {(current.memoryTotal / 1024).toFixed(1)} GB total
                </span>
              }
            />

            {/* A single "68% used" bar is the most misleading number a monitor
                can show. Page cache counts as used but is handed back the
                moment anything asks for it, so the split between genuinely
                committed memory and reclaimable cache is the difference
                between a healthy host and one about to swap. */}
            <div className="bg-panel-strong border-line mt-5 flex h-3 w-full overflow-hidden rounded-full border">
              <motion.div
                className="bg-ok"
                initial={false}
                animate={{ width: `${appPct}%` }}
                transition={{ type: 'spring', stiffness: 180, damping: 26 }}
                title={`Applications: ${(appMB / 1024).toFixed(2)} GB`}
              />
              <motion.div
                className="bg-info"
                initial={false}
                animate={{ width: `${cachePct}%` }}
                transition={{ type: 'spring', stiffness: 180, damping: 26 }}
                title={`Cache and buffers: ${(cacheMB / 1024).toFixed(2)} GB`}
              />
            </div>

            <dl className="mt-4 space-y-2.5 text-[12px]">
              {[
                {
                  key: 'app',
                  swatch: 'bg-ok',
                  label: 'Applications',
                  value: appMB,
                  pct: appPct,
                  muted: false,
                },
                {
                  key: 'cache',
                  swatch: 'bg-info',
                  label: 'Pagecache & buffers',
                  value: cacheMB,
                  pct: cachePct,
                  muted: false,
                },
                {
                  key: 'free',
                  swatch: 'bg-panel-strong border-line border',
                  label: 'Available',
                  value: Math.max(0, current.memoryTotal - appMB - cacheMB),
                  pct: Math.max(0, 100 - appPct - cachePct),
                  muted: true,
                },
              ].map((row) => (
                <div key={row.key} className="flex items-center justify-between gap-3">
                  <dt
                    className={cn(
                      'flex items-center gap-2',
                      row.muted ? 'text-mute' : 'text-foreground',
                    )}
                  >
                    <span className={cn('size-2 shrink-0 rounded-sm', row.swatch)} aria-hidden="true" />
                    {row.label}
                  </dt>
                  <dd
                    className={cn(
                      'tabular font-mono font-semibold',
                      row.muted ? 'text-mute' : 'text-foreground',
                    )}
                  >
                    {(row.value / 1024).toFixed(2)} GB
                    <span className="text-melt ml-2 font-normal">{row.pct.toFixed(1)}%</span>
                  </dd>
                </div>
              ))}
            </dl>

            {!hasMemoryComposition && !loading && (
              <p className="text-mute mt-3 text-[11px] leading-relaxed">
                This host reports no cache or buffer breakdown, so everything in use is shown as
                application memory.
              </p>
            )}

            <div className="border-line mt-4 space-y-3 border-t pt-4">
              <div>
                <div className="mb-1.5 flex items-baseline justify-between text-[11px]">
                  <span className="text-foreground font-medium">Swap</span>
                  <span className="text-mute tabular">
                    {swapPct.toFixed(0)}% · {current.swapUsed.toFixed(0)} of{' '}
                    {current.swapTotal.toFixed(0)} MB
                  </span>
                </div>
                <Meter value={swapPct} tone={swapTone} label="Swap utilisation" />
              </div>
              <div>
                <div className="mb-1.5 flex items-baseline justify-between text-[11px]">
                  <span className="text-foreground font-medium">CPU</span>
                  <span className="text-mute tabular">
                    {current.cpuLoad.toFixed(0)}% · {current.cpuPerCore.length} cores
                  </span>
                </div>
                <Meter value={current.cpuLoad} tone={cpuTone} label="CPU utilisation" />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="lg:col-span-3">
          <CardHeader>
            <CardTitle>Per-core utilisation</CardTitle>
            {current.cpuPerCore.length > 0 && (
              <span className="text-mute ml-auto font-mono text-[11px]">
                {current.cpuPerCore.length} cores
              </span>
            )}
          </CardHeader>
          <CardContent>
            <CoreGrid cores={current.cpuPerCore} />
          </CardContent>
        </Card>
      </div>

      {/* Capacity with real denominators. A percentage alone is not actionable
          — "81%" of a 500 GB volume and of a 100 TB volume are different
          problems with different response times. */}
      <Card>
        <CardContent className="p-5">
          <SectionHeading
            eyebrow="Capacity truth"
            title="Mounted filesystems"
            action={
              <span className="text-mute font-mono text-[10px]">
                {current.filesystems.length} mounts
              </span>
            }
          />
          {current.filesystems.length === 0 ? (
            <p className="text-mute mt-5 text-xs">No filesystem capacity reported.</p>
          ) : (
            <div className="mt-4 overflow-x-auto">
              <table className="w-full min-w-[680px] text-left text-[12px]">
                <thead className="text-mute font-mono text-[10px] tracking-[0.15em] uppercase">
                  <tr className="border-line border-b">
                    <th className="pb-3 font-medium">Mount</th>
                    <th className="pb-3 font-medium">Device</th>
                    <th className="pb-3 font-medium">Used</th>
                    <th className="pb-3 font-medium">Usage</th>
                    <th className="pb-3 text-right font-medium">Inodes</th>
                  </tr>
                </thead>
                <tbody className="font-mono">
                  {current.filesystems.map((fs) => {
                    const tone = toneFor(fs.usedPercent, 80, 90);
                    return (
                      <tr key={fs.mountpoint} className="border-line hover:bg-panel-strong/60 border-b last:border-0">
                        <td className="text-foreground py-3 font-medium">{fs.mountpoint}</td>
                        <td className="text-mute py-3">
                          {fs.device}{' '}
                          <span className="border-line bg-panel rounded border px-1 py-0.5 text-[10px]">
                            {fs.fstype}
                          </span>
                        </td>
                        <td className="text-mute tabular py-3">
                          {formatBytes(fs.usedBytes)} / {formatBytes(fs.totalBytes)}
                        </td>
                        <td className="py-3">
                          <div className="flex items-center gap-2">
                            <Meter
                              value={fs.usedPercent}
                              tone={tone}
                              className="w-20"
                              label={`${fs.mountpoint} utilisation`}
                            />
                            <span
                              className={cn(
                                'tabular font-semibold',
                                tone === 'crit'
                                  ? 'text-crit'
                                  : tone === 'warn'
                                    ? 'text-warn'
                                    : 'text-mute',
                              )}
                            >
                              {fs.usedPercent.toFixed(1)}%
                            </span>
                          </div>
                        </td>
                        <td className="text-mute tabular py-3 text-right">
                          {fs.inodesUsedPercent.toFixed(1)}%
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <SystemChart title="CPU usage" data={metricsHistory} dataKey="cpuLoad" color="var(--chart-1)" unit="%" maxValue={100} />
        <SystemChart title="Memory used" data={metricsHistory} dataKey="memoryUsed" color="var(--chart-2)" unit=" MB" />
        <SystemChart title="Disk write" data={metricsHistory} dataKey="diskWrite" color="var(--chart-3)" unit=" MB/s" />
        <SystemChart title="Network TX" data={metricsHistory} dataKey="networkTx" color="var(--chart-5)" unit=" KB/s" />
      </div>
    </div>
  );
};

export default Overview;
