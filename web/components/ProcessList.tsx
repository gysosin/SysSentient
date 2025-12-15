import React from 'react';
import { Process } from '../types';

interface ProcessListProps {
  processes: Process[];
}

const ProcessList: React.FC<ProcessListProps> = ({ processes }) => {
  return (
    <div className="bg-gray-900/90 backdrop-blur border border-gray-700 h-full flex flex-col relative overflow-hidden">
       {/* Header */}
       <div className="p-3 border-b border-gray-700 flex justify-between items-center bg-black/20">
         <h3 className="text-neon-blue text-xs font-bold uppercase tracking-widest flex items-center gap-2">
           <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M19.428 15.428a2 2 0 00-1.022-.547l-2.384-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z"></path></svg>
           Active_Processes
         </h3>
         <span className="text-[10px] text-gray-500 font-mono">PID_COUNT: {processes.length}</span>
       </div>

      <div className="overflow-auto flex-grow scrollbar-hide">
        <table className="w-full text-left text-sm text-gray-400">
          <thead className="bg-gray-950 text-gray-500 sticky top-0 z-10 text-[10px] uppercase tracking-wider font-mono">
            <tr>
              <th className="px-4 py-2 border-b border-gray-800">PID</th>
              <th className="px-4 py-2 border-b border-gray-800">Process_Name</th>
              <th className="px-4 py-2 border-b border-gray-800">User</th>
              <th className="px-4 py-2 border-b border-gray-800 text-right">CPU</th>
              <th className="px-4 py-2 border-b border-gray-800 text-right">MEM</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800/50">
            {processes.map((proc) => (
              <tr key={proc.pid} className="group hover:bg-neon-blue/5 transition-colors cursor-default">
                <td className="px-4 py-2 font-mono text-neon-blue/70 group-hover:text-neon-blue">{proc.pid}</td>
                <td className="px-4 py-2 font-medium text-gray-200 group-hover:text-white group-hover:translate-x-1 transition-transform">{proc.name}</td>
                <td className="px-4 py-2 text-xs">{proc.user}</td>
                <td className={`px-4 py-2 text-right font-mono ${proc.cpu > 50 ? 'text-neon-red animate-pulse' : 'text-neon-green'}`}>
                  {proc.cpu.toFixed(1)}%
                </td>
                <td className="px-4 py-2 text-right font-mono text-gray-400">{proc.memory}MB</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};

export default ProcessList;