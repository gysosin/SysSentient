import { SystemMetrics, Process, LogEntry } from '../types';

let currentMetrics: SystemMetrics = {
  timestamp: Date.now(),
  cpuLoad: 15,
  memoryUsed: 4096,
  memoryTotal: 16384,
  diskRead: 0.5,
  diskWrite: 1.2,
  networkRx: 150,
  networkTx: 50,
  temperature: 45
};

const PROCESS_NAMES = [
  'chrome', 'dockerd', 'node', 'code', 'slack', 'spotify', 'gnome-shell', 'Xorg', 'systemd', 'kworker'
];

const USERS = ['root', 'jdoe', 'service-acct'];

let isStressed = false;

export const toggleStressMode = (stressed: boolean) => {
  isStressed = stressed;
};

export const getSimulatedMetrics = (): SystemMetrics => {
  const now = Date.now();
  
  if (isStressed) {
    // Simulate high load
    currentMetrics = {
      timestamp: now,
      cpuLoad: Math.min(100, currentMetrics.cpuLoad + Math.random() * 10),
      memoryUsed: Math.min(16384, currentMetrics.memoryUsed + Math.random() * 500),
      memoryTotal: 16384,
      diskRead: Math.random() * 100,
      diskWrite: Math.random() * 200,
      networkRx: Math.random() * 5000,
      networkTx: Math.random() * 2000,
      temperature: Math.min(95, currentMetrics.temperature + Math.random() * 2)
    };
  } else {
    // Normal fluctuations
    currentMetrics = {
      timestamp: now,
      cpuLoad: Math.max(5, Math.min(100, currentMetrics.cpuLoad + (Math.random() - 0.5) * 10)),
      memoryUsed: Math.max(2000, Math.min(16384, currentMetrics.memoryUsed + (Math.random() - 0.5) * 200)),
      memoryTotal: 16384,
      diskRead: Math.max(0, Math.random() * 5),
      diskWrite: Math.max(0, Math.random() * 10),
      networkRx: Math.max(0, Math.random() * 500),
      networkTx: Math.max(0, Math.random() * 200),
      temperature: Math.max(35, Math.min(90, currentMetrics.temperature + (Math.random() - 0.5) * 2))
    };
  }

  return { ...currentMetrics };
};

export const getSimulatedProcesses = (): Process[] => {
  const processes: Process[] = [];
  
  // Always include a few heavy hitters if stressed
  if (isStressed) {
    processes.push({
      pid: 1234,
      name: 'node',
      user: 'jdoe',
      cpu: 85.5,
      memory: 4096,
      state: 'Running'
    });
    processes.push({
      pid: 5678,
      name: 'dockerd',
      user: 'root',
      cpu: 45.2,
      memory: 2048,
      state: 'Running'
    });
  }

  // Fill the rest with random processes
  const count = 10;
  for (let i = 0; i < count; i++) {
    const name = PROCESS_NAMES[Math.floor(Math.random() * PROCESS_NAMES.length)];
    processes.push({
      pid: Math.floor(Math.random() * 30000) + 1000,
      name,
      user: USERS[Math.floor(Math.random() * USERS.length)],
      cpu: parseFloat((Math.random() * (isStressed ? 10 : 5)).toFixed(1)),
      memory: Math.floor(Math.random() * 500),
      state: 'Running'
    });
  }

  return processes.sort((a, b) => b.cpu - a.cpu);
};

export const getSimulatedLogs = (): LogEntry[] => {
  if (!isStressed) return [];

  return [
    {
      timestamp: new Date().toISOString(),
      facility: 'kernel',
      level: 'ERROR',
      message: 'Out of memory: Kill process 1234 (node) score 855 or sacrifice child'
    },
    {
      timestamp: new Date().toISOString(),
      facility: 'systemd',
      level: 'WARN',
      message: 'Unit docker.service entered failed state.'
    },
    {
      timestamp: new Date().toISOString(),
      facility: 'kernel',
      level: 'WARN',
      message: 'CPU1: Core temperature above threshold, cpu clock throttled (total events = 422)'
    }
  ];
};
