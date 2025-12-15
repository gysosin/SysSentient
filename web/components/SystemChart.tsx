import React from 'react';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer
} from 'recharts';
import { SystemMetrics } from '../types';

interface SystemChartProps {
  data: SystemMetrics[];
  dataKey: keyof SystemMetrics;
  color: string;
  title: string;
  unit: string;
  maxValue?: number;
}

const SystemChart: React.FC<SystemChartProps> = ({ data, dataKey, color, title, unit, maxValue }) => {
  const gradientId = `gradient-${dataKey}`;

  return (
    <div className="bg-gray-900/80 backdrop-blur-sm p-4 border border-gray-800 shadow-lg flex flex-col h-64 relative overflow-hidden group">
      {/* Tech corners */}
      <div className="absolute top-0 left-0 w-2 h-2 border-t border-l border-gray-500"></div>
      <div className="absolute top-0 right-0 w-2 h-2 border-t border-r border-gray-500"></div>
      <div className="absolute bottom-0 left-0 w-2 h-2 border-b border-l border-gray-500"></div>
      <div className="absolute bottom-0 right-0 w-2 h-2 border-b border-r border-gray-500"></div>

      <div className="flex justify-between items-center mb-2">
        <h3 className="text-gray-400 text-xs font-bold uppercase tracking-wider flex items-center gap-2">
          <span className="w-1.5 h-1.5 rounded-full bg-current" style={{color}}></span>
          {title}
        </h3>
        <span className="text-[10px] text-gray-600 font-mono">LIVE_FEED_01</span>
      </div>
      
      <div className="flex-grow relative">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data}>
            <defs>
              <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={color} stopOpacity={0.3}/>
                <stop offset="95%" stopColor={color} stopOpacity={0}/>
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" vertical={false} />
            <XAxis 
              dataKey="timestamp" 
              tick={false} 
              axisLine={false}
            />
            <YAxis 
              domain={[0, maxValue || 'auto']} 
              stroke="#4b5563" 
              tick={{ fontSize: 10, fill: '#6b7280' }}
              width={35}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip 
              contentStyle={{ 
                backgroundColor: 'rgba(17, 24, 39, 0.95)', 
                borderColor: color, 
                color: '#f3f4f6',
                borderWidth: '1px',
                borderRadius: '0px',
                boxShadow: `0 0 10px ${color}33`,
                fontFamily: 'JetBrains Mono'
              }}
              itemStyle={{ color: color }}
              labelFormatter={() => ''}
              formatter={(value: number) => [`${value.toFixed(1)}${unit}`, title]}
              cursor={{ stroke: color, strokeWidth: 1, strokeDasharray: '4 4' }}
            />
            <Area 
              type="monotone" 
              dataKey={dataKey} 
              stroke={color} 
              fill={`url(#${gradientId})`}
              strokeWidth={2} 
              isAnimationActive={true}
              animationDuration={500}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};

export default SystemChart;