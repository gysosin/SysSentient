export interface Process {
  pid: number;
  name: string;
  user: string;
  cpu: number;
  memory: number; // in MB
  state: 'Running' | 'Sleeping' | 'Zombie' | 'Stopped';
}

export interface SystemMetrics {
  timestamp: number;
  cpuLoad: number;
  cpuPerCore: number[];
  memoryUsed: number; // in MB
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
