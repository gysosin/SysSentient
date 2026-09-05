import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import {
  AIAnalysisResult,
  FeedStatus,
  FleetHost,
  LogEntry,
  Process,
  SystemMetrics,
} from '../types';
import {
  fetchActiveAlerts,
  fetchHosts,
  fetchInsightHistory,
  fetchMetricsRange,
  type InsightEntry,
  fetchMetricsHistory,
  fetchRecentLogs,
  triggerAnalysis,
} from '../services/api';
import { useWebSocket } from './useWebSocket';
import { INSIGHT_REFRESH_RATE_MS, LOG_REFRESH_RATE_MS, REFRESH_RATE_MS } from '../constants';
import { resolveBounds, useTimeRange, type TimeRange, type RangePreset } from './useTimeRange';
import { usePageVisible } from './usePageVisible';

// A feed is stale once it has missed several poll intervals. This is what makes
// a half-open socket visible: the badge can read LIVE while no frame has
// arrived for minutes, and the dashboard used to keep presenting the last value
// as if it were current.
const STALE_AFTER_MS = REFRESH_RATE_MS * 5;

// Points retained for the charts. Matches the daemon's default page size so
// a seeded history and the live stream line up.
const MAX_HISTORY = 120;

export const EMPTY_METRIC: SystemMetrics = {
  processCount: 0,
  hostId: '',
  hostname: '',
  timestamp: 0,
  cpuLoad: 0,
  cpuPerCore: [],
  memoryUsed: 0,
  memoryTotal: 0,
  memoryCached: 0,
  memoryBuffers: 0,
  swapUsed: 0,
  swapTotal: 0,
  diskRead: 0,
  diskWrite: 0,
  diskIOPS: 0,
  networkRx: 0,
  networkTx: 0,
  loadAvg1: 0,
  loadAvg5: 0,
  loadAvg15: 0,
  temperature: 0,
  uptimeSeconds: 0,
  filesystems: [],
};

interface DashboardData {
  /** Fleet inventory. Empty in a single-node install. */
  hosts: FleetHost[];
  /** Count of firing alerts, surfaced as a badge in the navigation. */
  firingAlerts: number;
  /** Currently selected host id; empty means "all hosts". */
  selectedHost: string;
  selectHost: (hostID: string) => void;
  metricsHistory: SystemMetrics[];
  current: SystemMetrics;
  hasData: boolean;
  processes: Process[];
  logs: LogEntry[];
  /** True while the stream is deliberately held still for inspection. */
  frozen: boolean;
  toggleFreeze: () => void;
  /** The window every chart draws. */
  range: TimeRange;
  selectRange: (preset: RangePreset) => void;
  /** Zoom to an explicit window, from a drag over a chart. */
  zoomRange: (from: Date, to: Date) => void;
  resetRange: () => void;
  /** Which storage tier answered the current window: raw, 1m or 5m. */
  rangeResolution: string;
  ai: {
    result: AIAnalysisResult | null;
    /** Every stored analysis, newest first. */
    history: InsightEntry[];
    loading: boolean;
    error: string | null;
    run: () => Promise<void>;
  };
}

const DashboardContext = createContext<DashboardData | null>(null);

/**
 * The feed badge, on its own context because it ticks every second.
 *
 * It used to live in the main context value, so a tick published a new object
 * to all eleven consumers -- four charts among them -- purely to update the
 * "updated Ns ago" string that two small pieces of UI display.
 */
const FeedContext = createContext<FeedStatus | null>(null);

/**
 * Subscribes to the feed badge alone.
 *
 * A component that calls this re-renders every second by design. Keep those
 * components as small as possible, and never call it from one that renders
 * charts or tables.
 */
export function useFeed(): FeedStatus {
  const ctx = useContext(FeedContext);
  if (!ctx) {
    throw new Error('useFeed must be used inside <DashboardProvider>');
  }
  return ctx;
}

export function useDashboard(): DashboardData {
  const ctx = useContext(DashboardContext);
  if (!ctx) {
    throw new Error('useDashboard must be used inside <DashboardProvider>');
  }
  return ctx;
}

export const DashboardProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  // Nothing below runs while the tab is hidden; see usePageVisible.
  const visible = usePageVisible();
  const {
    connected,
    metricsHistory: wsMetricsHistory,
    processes: wsProcesses,
  } = useWebSocket(visible);

  const [metricsHistory, setMetricsHistory] = useState<SystemMetrics[]>([]);
  // Only the polling fallback writes this; the live socket supplies its own.
  const [polledProcesses, setPolledProcesses] = useState<Process[]>([]);
  const [insightHistory, setInsightHistory] = useState<InsightEntry[]>([]);
  const { range, selectPreset, zoomTo, reset, isLive } = useTimeRange();
  const [rangeMetrics, setRangeMetrics] = useState<SystemMetrics[]>([]);
  const [rangeResolution, setRangeResolution] = useState('raw');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [hosts, setHosts] = useState<FleetHost[]>([]);
  const [firingAlerts, setFiringAlerts] = useState(0);
  const [selectedHost, setSelectedHost] = useState('');
  const [now, setNow] = useState(() => Date.now());

  // Freeze holds a snapshot rather than pausing collection. Stopping the
  // pollers would make the feed look stale, and "the operator asked me to hold
  // still" must never be confused with "the machine stopped answering" — that
  // distinction is the whole trust model of this console. Data keeps arriving
  // underneath; only the view is pinned.
  const [frozen, setFrozen] = useState<{
    history: SystemMetrics[];
    processes: Process[];
    logs: LogEntry[];
  } | null>(null);

  const [aiResult, setAiResult] = useState<AIAnalysisResult | null>(null);
  const [isAiLoading, setIsAiLoading] = useState(false);
  const [aiError, setAiError] = useState<string | null>(null);

  // Seed history once per host selection so charts have shape immediately.
  // The socket's own buffer starts empty, so relying on it alone collapsed the
  // charts to a single point on every page load.
  useEffect(() => {
    let cancelled = false;
    const seed = async () => {
      const { metrics } = await fetchMetricsHistory(selectedHost);
      if (cancelled || metrics.length === 0) return;
      setMetricsHistory((prev) => (prev.length >= metrics.length ? prev : metrics));
    };
    seed();
    return () => {
      cancelled = true;
    };
  }, [selectedHost]);

  useEffect(() => {
    if (!connected || wsMetricsHistory.length === 0) return;

    // The socket streams the server's own view. When a specific host is
    // selected, only its samples belong on screen.
    const incoming = selectedHost
      ? wsMetricsHistory.filter((m) => m.hostId === selectedHost || m.hostId === '')
      : wsMetricsHistory;
    if (incoming.length === 0) return;

    // Append only frames newer than what we already hold, so the seeded
    // history survives instead of being replaced by the socket's short buffer.
    setMetricsHistory((prev) => {
      if (prev.length === 0) return incoming;
      const newestSeen = prev[prev.length - 1].timestamp;
      const fresh = incoming.filter((m) => m.timestamp > newestSeen);
      if (fresh.length === 0) return prev;
      return [...prev, ...fresh].slice(-MAX_HISTORY);
    });
  }, [connected, wsMetricsHistory, selectedHost]);

  /**
   * Stored analyses, loaded regardless of transport.
   *
   * This used to live inside the polling fallback, which returns early when
   * the socket is connected — so with a healthy WebSocket the dashboard never
   * loaded a single stored insight and reported "No analysis yet" against a
   * database full of them.
   */
  useEffect(() => {
    if (!visible) return;
    let cancelled = false;
    const load = async () => {
      const history = await fetchInsightHistory(selectedHost);
      if (cancelled) return;
      setInsightHistory(history);
      if (history.length > 0) {
        setAiResult((prev) => prev ?? history[0].analysis);
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), INSIGHT_REFRESH_RATE_MS);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [selectedHost]);

  /**
   * Bounded window query.
   *
   * Runs only when a window is selected. While live the socket keeps driving
   * the view, which is the behaviour the console has always had and what an
   * operator watching a machine right now wants.
   */
  useEffect(() => {
    if (isLive) {
      setRangeMetrics([]);
      return;
    }
    const bounds = resolveBounds(range);
    if (!bounds) return;

    let cancelled = false;
    void (async () => {
      const result = await fetchMetricsRange(bounds.from, bounds.to, selectedHost);
      if (cancelled || !result) return;
      setRangeMetrics(result.metrics);
      setRangeResolution(result.resolution);
    })();
    return () => {
      cancelled = true;
    };
  }, [isLive, range, selectedHost, visible]);

  // Fallback polling when the WebSocket is not connected.
  useEffect(() => {
    if (connected || !visible) return;
    let cancelled = false;

    const fetchData = async () => {
      const { metrics, processes: procs } = await fetchMetricsHistory(selectedHost);
      if (cancelled) return;
      // Assigned unconditionally: guarding on `length > 0` left the last
      // known-good numbers on screen through an outage, indistinguishable from
      // a healthy idle system.
      setMetricsHistory(metrics);
      setPolledProcesses(procs);

    };

    fetchData();
    const intervalId = setInterval(fetchData, REFRESH_RATE_MS);
    return () => {
      cancelled = true;
      clearInterval(intervalId);
    };
  }, [connected, selectedHost, visible]);

  useEffect(() => {
    if (!visible) return;
    let cancelled = false;
    const fetchLogs = async () => {
      const recentLogs = await fetchRecentLogs();
      if (cancelled) return;
      setLogs(recentLogs);
    };

    fetchLogs();
    const intervalId = setInterval(fetchLogs, LOG_REFRESH_RATE_MS);
    return () => {
      cancelled = true;
      clearInterval(intervalId);
    };
  }, [visible]);

  // Firing-alert count for the nav badge. Cheap; polled on a slow cadence.
  useEffect(() => {
    if (!visible) return;
    let cancelled = false;
    const load = async () => {
      const alerts = await fetchActiveAlerts(selectedHost);
      if (!cancelled) setFiringAlerts(alerts.filter((a) => a.state === 'firing').length);
    };
    load();
    const id = setInterval(load, 10000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [selectedHost, visible]);

  // Fleet inventory. Cheap and slow-moving, so polled well below metric rate.
  useEffect(() => {
    if (!visible) return;
    let cancelled = false;
    const load = async () => {
      const list = await fetchHosts();
      if (!cancelled) setHosts(list);
    };
    load();
    const id = setInterval(load, 15000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [visible]);

  // A selected host that stops reporting must not strand the dashboard on an
  // empty view forever.
  useEffect(() => {
    if (selectedHost && hosts.length > 0 && !hosts.some((h) => h.hostId === selectedHost)) {
      setSelectedHost('');
    }
  }, [hosts, selectedHost]);

  // Drives the "updated Ns ago" readout and staleness detection.
  useEffect(() => {
    if (!visible) return;
    const interval = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [visible]);

  const run = async () => {
    setIsAiLoading(true);
    setAiError(null);
    try {
      setAiResult(await triggerAnalysis());
    } catch (error) {
      setAiError(error instanceof Error ? error.message : 'AI analysis failed');
    } finally {
      setIsAiLoading(false);
    }
  };

  const liveProcesses = connected ? wsProcesses : polledProcesses;

  // What a freeze would capture, kept in a ref so the callback below can have
  // a stable identity. Its dependencies changed on every frame, which made it
  // a new function twice a second and re-published the context on its own.
  const snapshotRef = useRef({ history: metricsHistory, processes: liveProcesses, logs });
  useEffect(() => {
    snapshotRef.current = { history: metricsHistory, processes: liveProcesses, logs };
  });

  const toggleFreeze = useCallback(() => {
    setFrozen((prev) => (prev ? null : snapshotRef.current));
  }, []);

  const value = useMemo<DashboardData>(() => {
    // A selected window replaces the live tail. Freeze still wins, because
    // it is an explicit request to stop the view moving.
    const shownHistory = frozen
      ? frozen.history
      : isLive
        ? metricsHistory
        : rangeMetrics;
    const hasData = shownHistory.length > 0;
    const current = hasData ? shownHistory[shownHistory.length - 1] : EMPTY_METRIC;
    return {
      hosts,
      firingAlerts,
      selectedHost,
      selectHost: setSelectedHost,
      metricsHistory: shownHistory,
      current,
      hasData,
      processes: frozen ? frozen.processes : liveProcesses,
      logs: frozen ? frozen.logs : logs,
      frozen: frozen !== null,
      toggleFreeze,
      range,
      selectRange: selectPreset,
      zoomRange: zoomTo,
      resetRange: reset,
      rangeResolution,
      ai: { result: aiResult, history: insightHistory, loading: isAiLoading, error: aiError, run },
    };
    // `run` is stable enough for this provider's lifetime; excluded deliberately.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [metricsHistory, connected, liveProcesses, logs, aiResult, insightHistory, isAiLoading, aiError, range, selectPreset, zoomTo, reset, rangeResolution, isLive, rangeMetrics, hosts, selectedHost, firingAlerts, frozen, toggleFreeze]);

  /**
   * The feed badge, republished every second by the clock tick.
   *
   * Deliberately a separate context. This used to live in the main value, so a
   * tick published a new object to all eleven consumers -- four charts among
   * them -- to update a string that only the header chip and the hero line
   * display. Anything that needs the age now subscribes here instead, and the
   * rest of the console re-renders only when its data actually changes.
   */
  const feed = useMemo<FeedStatus>(() => {
    // Measured against the live sample even while frozen, so the badge keeps
    // telling the truth about the daemon rather than about the snapshot.
    const liveCurrent = metricsHistory.length > 0 ? metricsHistory[metricsHistory.length - 1] : null;
    const ageMs = liveCurrent ? Math.max(0, now - liveCurrent.timestamp) : Infinity;
    const ageSeconds = Math.round(ageMs / 1000);

    if (!liveCurrent) {
      return { level: 'down', label: 'NO DATA', detail: 'awaiting first sample', ageMs };
    }
    if (ageMs > STALE_AFTER_MS) {
      return { level: 'stale', label: 'STALE', detail: `no update for ${ageSeconds}s`, ageMs };
    }
    if (connected) {
      return { level: 'live', label: 'LIVE', detail: `updated ${ageSeconds}s ago`, ageMs };
    }
    return { level: 'polling', label: 'POLLING', detail: `updated ${ageSeconds}s ago`, ageMs };
  }, [metricsHistory, now, connected]);

  return (
    <DashboardContext.Provider value={value}>
      <FeedContext.Provider value={feed}>{children}</FeedContext.Provider>
    </DashboardContext.Provider>
  );
};
