import React, { useState, useEffect, useRef } from 'react';
import { SystemMetrics, Process, LogEntry, AIAnalysisResult } from './types';
import { fetchMetricsHistory, triggerAnalysis, fetchLatestInsight } from './services/api';
import { useWebSocket } from './hooks/useWebSocket';
import SystemChart from './components/SystemChart';
import ProcessList from './components/ProcessList';
import AIInsightPanel from './components/AIInsightPanel';
import { REFRESH_RATE_MS } from './constants';

const App: React.FC = () => {
  const { connected, metricsHistory: wsMetricsHistory } = useWebSocket();
  const [metricsHistory, setMetricsHistory] = useState<SystemMetrics[]>([]);
  const [processes, setProcesses] = useState<Process[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [uptime, setUptime] = useState(0);

  // AI State
  const [aiResult, setAiResult] = useState<AIAnalysisResult | null>(null);
  const [isAiLoading, setIsAiLoading] = useState(false);

  const logsEndRef = useRef<HTMLDivElement>(null);

  // Use WebSocket metrics if connected, otherwise fall back to polling
  useEffect(() => {
    if (connected && wsMetricsHistory.length > 0) {
      setMetricsHistory(wsMetricsHistory);
    }
  }, [connected, wsMetricsHistory]);

  // Fallback polling when WebSocket is not connected
  useEffect(() => {
    if (connected) return; // Skip polling if WebSocket is active

    const fetchData = async () => {
      const { metrics, processes: procs } = await fetchMetricsHistory();
      if (metrics.length > 0) {
        setMetricsHistory(metrics);
      }
      if (procs.length > 0) {
        setProcesses(procs);
      }

      const latestInsight = await fetchLatestInsight();
      if (latestInsight) {
        setAiResult(prev => prev ? prev : latestInsight);
      }
    };

    fetchData();
    const intervalId = setInterval(fetchData, REFRESH_RATE_MS);
    return () => clearInterval(intervalId);
  }, [connected]);

  // Uptime counter
  useEffect(() => {
    const interval = setInterval(() => {
      setUptime(prev => prev + 1);
    }, 1000);
    return () => clearInterval(interval);
  }, []);

  // Auto-scroll logs
  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  const handleRunDiagnostics = async () => {
    setIsAiLoading(true);
    const result = await triggerAnalysis();
    if (result) {
      setAiResult(result);
    }
    setIsAiLoading(false);
  };

  const currentMetric = metricsHistory.length > 0 ? metricsHistory[metricsHistory.length - 1] : {
    timestamp: Date.now(), cpuLoad: 0, cpuPerCore: [], memoryUsed: 0, memoryTotal: 16384,
    swapUsed: 0, swapTotal: 0, temperature: 0, diskRead: 0, diskWrite: 0, diskIOPS: 0,
    networkRx: 0, networkTx: 0, loadAvg1: 0, loadAvg5: 0, loadAvg15: 0
  };

  // Format uptime
  const formatUptime = (sec: number) => {
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = Math.floor(sec % 60);
    return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  };

  return (
    <div className="min-h-screen text-gray-200 p-2 lg:p-6 font-mono selection:bg-neon-purple selection:text-white">

      {/* Header */}
      <header className="flex flex-col md:flex-row justify-between items-start md:items-center mb-8 border-b border-gray-800 pb-4 relative">
        <div className="relative z-10">
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className={`w-3 h-3 ${connected ? 'bg-neon-green' : 'bg-neon-purple'} rounded-full`}></div>
              <div className={`absolute top-0 left-0 w-3 h-3 ${connected ? 'bg-neon-green' : 'bg-neon-purple'} rounded-full animate-ping`}></div>
            </div>
            <h1 className="text-3xl font-bold text-white tracking-tighter uppercase glitch-text" data-text="SysSentient">
              SysSentient
            </h1>
            <span className="px-2 py-0.5 border border-gray-600 text-[10px] text-gray-400 rounded bg-gray-900/50">V.3.0.0-WS</span>
            <span className={`px-2 py-0.5 text-[10px] rounded ${connected ? 'bg-neon-green/20 text-neon-green border border-neon-green/50' : 'bg-yellow-500/20 text-yellow-500 border border-yellow-500/50'}`}>
              {connected ? '● LIVE' : '○ POLLING'}
            </span>
          </div>
          <p className="text-neon-blue text-xs mt-1 tracking-widest uppercase opacity-70 ml-6">&gt; Intelligent Kernel Monitor Interface</p>
        </div>

        <div className="flex flex-col items-end gap-2 mt-4 md:mt-0 relative z-10">
          <div className="text-right">
            <span className="text-[10px] text-gray-500 uppercase tracking-widest mr-2">System_Uptime</span>
            <span className="font-mono text-neon-green text-lg">{formatUptime(uptime)}</span>
          </div>
        </div>
      </header>

      {/* Main Grid */}
      <main className="grid grid-cols-1 lg:grid-cols-12 gap-6 relative z-10">

        {/* Left Column (8/12) */}
        <div className="lg:col-span-8 space-y-6">
          {/* Top Stats Cards - Extended Grid */}
          <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-4">
            {[
              { label: 'CPU_Load', val: currentMetric.cpuLoad.toFixed(1) + '%', alert: currentMetric.cpuLoad > 80, color: 'text-neon-purple' },
              { label: 'RAM_Usage', val: (currentMetric.memoryUsed / 1024).toFixed(1) + 'GB', alert: false, color: 'text-neon-blue' },
              { label: 'Load_Avg', val: currentMetric.loadAvg1.toFixed(2), alert: currentMetric.loadAvg1 > 4, color: 'text-orange-400' },
              { label: 'Swap_Used', val: currentMetric.swapUsed.toFixed(0) + 'MB', alert: currentMetric.swapUsed > 1000, color: 'text-pink-400' },
              { label: 'Disk_IOPS', val: currentMetric.diskIOPS.toFixed(0), alert: false, color: 'text-cyan-400' },
              { label: 'Net_Traffic', val: currentMetric.networkRx.toFixed(0) + 'KB/s', alert: false, color: 'text-neon-green' }
            ].map((stat, i) => (
              <div key={i} className={`bg-gray-900/50 border ${stat.alert ? 'border-neon-red animate-pulse' : 'border-gray-800'} p-4 relative overflow-hidden group hover:border-gray-600 transition-colors`}>
                <div className={`absolute top-0 right-0 p-1 opacity-20 ${stat.color} group-hover:opacity-100 transition-opacity`}>
                  <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1" d="M13 10V3L4 14h7v7l9-11h-7z" /></svg>
                </div>
                <p className="text-[10px] text-gray-500 uppercase tracking-widest mb-1">{stat.label}</p>
                <p className={`text-xl font-bold font-mono ${stat.alert ? 'text-neon-red' : stat.color}`}>{stat.val}</p>
              </div>
            ))}
          </div>

          {/* Charts Area - 2x2 Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <SystemChart
              title="CPU_USAGE"
              data={metricsHistory}
              dataKey="cpuLoad"
              color="#bc13fe"
              unit="%"
              maxValue={100}
            />
            <SystemChart
              title="MEMORY_USAGE"
              data={metricsHistory}
              dataKey="memoryUsed"
              color="#00f3ff"
              unit=" MB"
            />
            <SystemChart
              title="DISK_IO_WRITE"
              data={metricsHistory}
              dataKey="diskWrite"
              color="#ff6b6b"
              unit=" MB/s"
            />
            <SystemChart
              title="NETWORK_RX"
              data={metricsHistory}
              dataKey="networkRx"
              color="#0aff00"
              unit=" KB/s"
            />
          </div>

          {/* Retro Terminal Logs */}
          <div className="bg-black border border-gray-800 rounded-sm p-4 h-48 overflow-hidden flex flex-col font-mono text-xs shadow-inner shadow-black">
            <div className="flex items-center gap-2 mb-2 border-b border-gray-800 pb-2">
              <span className="text-neon-green">➜</span>
              <span className="text-gray-400">~/var/log/syslog</span>
              <span className="ml-auto text-gray-600 text-[10px]">TAIL -F</span>
            </div>
            <div className="overflow-y-auto space-y-1 flex-grow scrollbar-hide">
              <div className="text-gray-700 italic">Log streaming API pending...<span className="animate-pulse">_</span></div>
              <div ref={logsEndRef} />
            </div>
          </div>
        </div>

        {/* Right Column (4/12) */}
        <div className="lg:col-span-4 flex flex-col gap-6">
          <div className="h-2/5 min-h-[350px]">
            <AIInsightPanel
              analysis={aiResult}
              loading={isAiLoading}
              onRefresh={handleRunDiagnostics}
            />
          </div>
          <div className="h-3/5 min-h-[400px]">
            <ProcessList processes={processes} />
          </div>
        </div>

      </main>
    </div>
  );
};

export default App;
