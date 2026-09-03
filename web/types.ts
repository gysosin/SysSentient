export interface Process {
  pid: number;
  name: string;
  user: string;
  cpu: number;
  memory: number; // in MB
  state: 'Running' | 'Sleeping' | 'Zombie' | 'Stopped';
}

export interface Filesystem {
  mountpoint: string;
  device: string;
  fstype: string;
  totalBytes: number;
  usedBytes: number;
  freeBytes: number;
  usedPercent: number;
  inodesUsedPercent: number;
}

export interface SystemMetrics {
  hostname: string;
  timestamp: number;
  cpuLoad: number;
  cpuPerCore: number[];
  memoryUsed: number; // in MB
  /** Reclaimable page cache, in MB. Splitting this out of "used" is what
   *  separates a host that is fine from one about to swap. */
  memoryCached: number;
  memoryBuffers: number; // in MB
  memoryTotal: number; // in MB
  swapUsed: number; // in MB
  swapTotal: number; // in MB
  diskRead: number; // MB/s
  diskWrite: number; // MB/s
  diskIOPS: number;
  networkRx: number; // KB/s
  networkTx: number; // KB/s
  loadAvg1: number;
  loadAvg5: number;
  loadAvg15: number;
  temperature: number; // Celsius
  uptimeSeconds: number; // host uptime reported by the daemon
  filesystems: Filesystem[];
}

export interface LogEntry {
  timestamp: string;
  facility: string;
  level: 'INFO' | 'WARN' | 'ERROR';
  message: string;
}

export interface AIAction {
  id: string;
  command: string;
  description: string;
  isSafe: boolean; // true = auto-approvable, false = risky
}

export interface AIAnalysisResult {
  status: 'Healthy' | 'Warning' | 'Critical';
  summary: string;
  detailedAnalysis: string;
  recommendedActions: AIAction[];
}

/** How trustworthy the currently displayed data is. The old dashboard could not
 *  distinguish healthy from degraded from dead — every failure looked the same. */
export interface FeedStatus {
  level: 'live' | 'polling' | 'stale' | 'down';
  label: string;
  detail: string;
  ageMs: number;
}

export type AlertState = 'pending' | 'firing' | 'resolved';
export type AlertSeverity = 'warning' | 'critical';

export interface Alert {
  ruleId: string;
  ruleName: string;
  metric: string;
  state: AlertState;
  severity: AlertSeverity;
  value: number;
  threshold: number;
  hostname: string;
  startedAt: string;
  acknowledged: boolean;
}

export interface AlertRule {
  id: string;
  name: string;
  metric: string;
  op: string;
  threshold: number;
  forLabel: string;
  severity: AlertSeverity;
  enabled: boolean;
}

export interface AlertEvent {
  occurredAt: string;
  ruleId: string;
  ruleName: string;
  metric: string;
  state: AlertState;
  severity: AlertSeverity;
  value: number;
  threshold: number;
  hostname: string;
}

export interface HealthStatus {
  status: string;
  service: string;
  database: string;
  version: string;
  commit?: string;
  collector?: string;
  lastSampleAgeSeconds?: number;
  websocketClients?: number;
}

export interface FleetHost {
  hostId: string;
  hostname: string;
  firstSeen: string;
  lastSeen: string;
  agentVersion: string;
}
