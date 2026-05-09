import React from 'react';
import { AIAnalysisResult } from '../types';

interface AIInsightPanelProps {
  analysis: AIAnalysisResult | null;
  error: string | null;
  loading: boolean;
  onRefresh: () => void;
}

const AIInsightPanel: React.FC<AIInsightPanelProps> = ({ analysis, error, loading, onRefresh }) => {
  const [copiedActionId, setCopiedActionId] = React.useState<string | null>(null);
  const [copyErrorActionId, setCopyErrorActionId] = React.useState<string | null>(null);
  const analysisId = React.useMemo(() => {
    if (!analysis) return '';
    return Math.random().toString(36).slice(2, 8).toUpperCase();
  }, [analysis]);

  const handleCopyCommand = async (actionId: string, command: string) => {
    try {
      await navigator.clipboard.writeText(command);
      setCopiedActionId(actionId);
      setCopyErrorActionId(null);
      window.setTimeout(() => setCopiedActionId(null), 1500);
    } catch {
      setCopyErrorActionId(actionId);
      setCopiedActionId(null);
      window.setTimeout(() => setCopyErrorActionId(null), 1500);
    }
  };

  const getStatusColor = (status?: string) => {
    switch(status) {
      case 'Healthy': return 'border-neon-green text-neon-green shadow-[0_0_10px_rgba(10,255,0,0.2)]';
      case 'Warning': return 'border-yellow-500 text-yellow-500 shadow-[0_0_10px_rgba(234,179,8,0.2)]';
      case 'Critical': return 'border-neon-red text-neon-red shadow-[0_0_10px_rgba(255,0,60,0.2)]';
      default: return 'border-gray-600 text-gray-400';
    }
  };

  return (
    <div className="bg-gray-900/90 backdrop-blur border border-gray-700 h-full flex flex-col relative overflow-hidden group">
      {/* Decorative scan line */}
      <div className="absolute top-0 left-0 w-full h-0.5 bg-gradient-to-r from-transparent via-neon-purple to-transparent opacity-50"></div>

      {/* Header */}
      <div className="p-3 border-b border-gray-800 flex justify-between items-center bg-black/20">
        <div className="flex items-center gap-2">
            <div className={`w-2 h-2 rounded-full ${loading ? 'bg-neon-purple animate-ping' : 'bg-neon-purple'}`}></div>
            <h2 className="text-neon-purple font-bold text-sm tracking-widest uppercase">AI_Core_Analysis</h2>
        </div>
        <button
          onClick={onRefresh}
          disabled={loading}
          className={`
            relative px-4 py-1.5 text-xs font-bold uppercase tracking-wider transition-all border
            ${loading
              ? 'bg-gray-800 border-gray-700 text-gray-500 cursor-not-allowed'
              : 'bg-neon-purple/10 border-neon-purple text-neon-purple hover:bg-neon-purple hover:text-white hover:shadow-[0_0_15px_#bc13fe]'}
          `}
        >
          {loading ? 'Initializing...' : 'Execute_Scan'}
        </button>
      </div>

      <div className="p-5 flex-grow overflow-y-auto relative">
        {/* Background Grid inside panel */}
        <div className="absolute inset-0 opacity-5 pointer-events-none"
             style={{backgroundImage: 'radial-gradient(#bc13fe 1px, transparent 1px)', backgroundSize: '20px 20px'}}>
        </div>

        {!analysis && !loading && !error && (
          <div className="h-full flex flex-col items-center justify-center text-gray-600 space-y-4">
             <div className="w-16 h-16 border border-gray-700 rounded-full flex items-center justify-center relative">
                <div className="w-12 h-12 border border-gray-800 rounded-full animate-pulse"></div>
                <div className="absolute inset-0 border-t border-gray-600 rounded-full animate-spin duration-[3s]"></div>
             </div>
            <p className="font-mono text-xs uppercase tracking-widest">System_Standby // Awaiting Input</p>
          </div>
        )}

        {error && !loading && (
          <div className="relative z-10 mb-4 border border-neon-red/70 bg-neon-red/10 p-3 text-neon-red">
            <div className="text-[10px] uppercase tracking-widest opacity-80">Scan_Failed</div>
            <p className="mt-1 text-xs font-mono text-red-200">{error}</p>
          </div>
        )}

        {loading && (
          <div className="h-full flex flex-col items-center justify-center space-y-6">
            <div className="w-full max-w-[200px] space-y-2">
              <div className="flex justify-between text-xs text-neon-purple font-mono">
                <span>ANALYZING_METRICS</span>
                <span className="animate-pulse">...</span>
              </div>
              <div className="h-1 w-full bg-gray-800 overflow-hidden">
                <div className="h-full bg-neon-purple animate-[scan_1.5s_ease-in-out_infinite] w-1/2"></div>
              </div>
            </div>



                         <div className="font-mono text-[10px] text-gray-500 space-y-1">

                            <p className="animate-pulse">&gt; Reading /proc/meminfo...</p>

                            <p className="animate-pulse delay-75">&gt; Parsing kernel ring buffer...</p>

                            <p className="animate-pulse delay-150">&gt; Connecting to Neural Net...</p>

                         </div>

                       </div>


        )}

        {analysis && !loading && (
          <div className="space-y-6 relative z-10 animate-[fadeIn_0.3s_ease-out]">
            {/* Status HUD */}
            <div className={`p-4 border-l-4 ${getStatusColor(analysis.status)} bg-black/40`}>
               <div className="flex items-center justify-between mb-2">
                 <h3 className="font-bold text-xl uppercase tracking-widest">{analysis.status}</h3>
                 <span className="text-[10px] opacity-60">ID: {analysisId}</span>
               </div>
               <p className="text-sm font-mono opacity-90">{analysis.summary}</p>
            </div>

            {/* Analysis Terminal */}
            <div className="space-y-2">
               <h4 className="text-gray-500 text-[10px] font-bold uppercase tracking-widest border-b border-gray-800 pb-1">
                 Diagnostics_Output
               </h4>
               <p className="text-gray-300 text-sm leading-relaxed font-mono whitespace-pre-wrap break-words">
                 {analysis.detailedAnalysis}
                 <span className="inline-block w-2 h-4 bg-neon-purple ml-1 animate-pulse align-middle"></span>
               </p>
            </div>

            {/* Action Matrix */}
            {analysis.recommendedActions.length > 0 && (
              <div className="space-y-3">
                <h4 className="text-gray-500 text-[10px] font-bold uppercase tracking-widest border-b border-gray-800 pb-1">
                  Countermeasures
                </h4>
                {analysis.recommendedActions.map((action, idx) => (
                    <div key={action.id} style={{animationDelay: `${idx * 100}ms`}} className="group animate-[fadeIn_0.5s_ease-out_forwards] opacity-0">
                      <div className={`
                        border border-gray-700 bg-gray-900/50 p-3
                        ${action.isSafe ? 'hover:border-neon-green' : 'hover:border-neon-red'} transition-colors duration-300
                      `}>
                        <div className="flex justify-between items-start mb-2">
                           <span className={`text-[10px] px-2 py-0.5 border ${action.isSafe ? 'border-neon-green text-neon-green' : 'border-neon-red text-neon-red'} uppercase tracking-wider`}>
                             {action.isSafe ? 'Approved' : 'Restricted'}
                           </span>
                           <button
                             type="button"
                             onClick={() => handleCopyCommand(action.id, action.command)}
                             className="text-[10px] bg-gray-800 hover:bg-white hover:text-black px-3 py-1 uppercase tracking-wider transition-colors"
                           >
                             {copyErrorActionId === action.id ? 'Copy failed' : copiedActionId === action.id ? 'Copied' : 'Copy'}
                           </button>
                        </div>
                        <div className="bg-black p-2 font-mono text-sm text-neon-blue mb-2 border-l-2 border-gray-700 group-hover:border-neon-blue transition-colors whitespace-pre-wrap break-all">
                          $ {action.command}
                        </div>
                        <p className="text-gray-500 text-xs">{action.description}</p>
                      </div>
                    </div>
                  ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default AIInsightPanel;
