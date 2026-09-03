import React from 'react';
import { Sparkles } from 'lucide-react';
import { AIAnalysisResult } from '../types';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Skeleton } from './ui/skeleton';
import { cn } from '../lib/utils';

interface AIInsightPanelProps {
  analysis: AIAnalysisResult | null;
  error: string | null;
  loading: boolean;
  onRefresh: () => void;
}

const ANALYSIS_ID_LENGTH = 6;

function deriveAnalysisId(analysis: AIAnalysisResult): string {
  const source = JSON.stringify({
    status: analysis.status,
    summary: analysis.summary,
    detailedAnalysis: analysis.detailedAnalysis,
    recommendedActions: analysis.recommendedActions.map((action) => ({
      id: action.id,
      command: action.command,
      description: action.description,
      isSafe: action.isSafe,
    })),
  });
  let hash = 0x811c9dc5;

  for (let i = 0; i < source.length; i += 1) {
    hash ^= source.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }

  return (hash >>> 0)
    .toString(36)
    .toUpperCase()
    .padStart(ANALYSIS_ID_LENGTH, '0')
    .slice(-ANALYSIS_ID_LENGTH);
}

const statusTone = (status?: string) => {
  switch (status) {
    case 'Healthy':
      return 'border-ok/40 text-ok bg-ok-soft';
    case 'Warning':
      return 'border-warn/40 text-warn bg-warn-soft';
    case 'Critical':
      return 'border-crit/40 text-crit bg-crit-soft';
    default:
      return 'text-mute bg-panel-strong border-line';
  }
};

const AIInsightPanel: React.FC<AIInsightPanelProps> = ({ analysis, error, loading, onRefresh }) => {
  const [copiedActionId, setCopiedActionId] = React.useState<string | null>(null);
  const [copyErrorActionId, setCopyErrorActionId] = React.useState<string | null>(null);
  const analysisId = React.useMemo(() => (analysis ? deriveAnalysisId(analysis) : ''), [analysis]);

  const handleCopyCommand = async (actionId: string, command: string) => {
    try {
      // Unavailable on plain-HTTP origins, which is the common LAN case.
      if (!navigator.clipboard) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(command);
      setCopiedActionId(actionId);
      setCopyErrorActionId(null);
      window.setTimeout(() => setCopiedActionId(null), 1500);
    } catch {
      setCopyErrorActionId(actionId);
      setCopiedActionId(null);
      window.setTimeout(() => setCopyErrorActionId(null), 3000);
    }
  };

  return (
    <Card className="flex flex-col">
      <CardHeader>
        <Sparkles className="text-accent size-4" />
        <CardTitle>AI analysis</CardTitle>
        <Button variant="accent" size="sm" onClick={onRefresh} disabled={loading} className="ml-auto">
          {loading ? 'Analysing…' : 'Run analysis'}
        </Button>
      </CardHeader>

      <CardContent className="flex-grow space-y-5">
        {!analysis && !loading && !error && (
          <div className="py-14 text-center">
            <Sparkles className="text-melt mx-auto mb-3 size-7" />
            <p className="text-sm">No analysis yet</p>
            <p className="text-muted-foreground mt-1 text-xs">
              Runs automatically when CPU or memory crosses a threshold, or on demand.
            </p>
          </div>
        )}

        {error && !loading && (
          <div className="border-crit/40 bg-crit-soft rounded-lg border p-3.5" role="alert">
            <p className="text-crit mb-1 text-[11px] tracking-wide uppercase">Analysis failed</p>
            <p className="text-muted-foreground font-mono text-sm break-words">{error}</p>
          </div>
        )}

        {loading && (
          <div className="space-y-3 py-2" role="status" aria-live="polite">
            <p className="text-muted-foreground text-sm">
              Analysing current metrics and recent logs…
            </p>
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-4 w-4/5" />
            <Skeleton className="h-4 w-3/5" />
          </div>
        )}

        {analysis && !loading && (
          <div className="space-y-5 animate-[fadeIn_0.3s_ease-out]">
            <div className={cn('rounded-r-lg border-l-2 p-4', statusTone(analysis.status))}>
              <div className="mb-1 flex items-center justify-between gap-3">
                <h3 className="font-display text-lg font-bold tracking-[0.06em] uppercase">
                  {analysis.status}
                </h3>
                <span className="tabular font-mono text-[11px] opacity-70">#{analysisId}</span>
              </div>
              <p className="text-foreground text-sm">{analysis.summary}</p>
            </div>

            <div className="space-y-2">
              <h4 className="text-mute border-line border-b pb-1.5 text-[10px] font-semibold tracking-[0.2em] uppercase">
                Detail
              </h4>
              <p className="text-muted-foreground text-sm leading-relaxed break-words whitespace-pre-wrap">
                {analysis.detailedAnalysis}
              </p>
            </div>

            {analysis.recommendedActions.length > 0 && (
              <div className="space-y-3">
                <h4 className="text-mute border-line border-b pb-1.5 text-[10px] font-semibold tracking-[0.2em] uppercase">
                  Suggested commands
                </h4>

                {/* The "safe" flag is produced by the model itself and is not
                    validated by the daemon. Say so rather than implying review. */}
                <p className="text-warn text-xs">
                  Generated by a language model and not verified. Review before running.
                </p>

                {analysis.recommendedActions.map((action) => (
                  // No opacity-0 here. The previous version relied on a fadeIn
                  // keyframe that did not exist, so the whole list stayed
                  // permanently invisible.
                  <div
                    key={action.id}
                    className="bg-panel-strong border-line hover:border-accent/40 rounded-lg border p-3 transition-colors"
                  >
                    <div className="mb-2 flex items-start justify-between gap-2">
                      <Badge variant={action.isSafe ? 'ok' : 'crit'}>
                        {action.isSafe ? 'Model says: low risk' : 'Model says: destructive'}
                      </Badge>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleCopyCommand(action.id, action.command)}
                      >
                        {copyErrorActionId === action.id
                          ? 'Copy unavailable'
                          : copiedActionId === action.id
                            ? 'Copied'
                            : 'Copy'}
                      </Button>
                    </div>
                    <pre className="bg-background border-line text-foreground overflow-x-auto rounded-md border p-2.5 font-mono text-xs break-all whitespace-pre-wrap">
                      {action.command}
                    </pre>
                    <p className="text-mute mt-2 text-xs">{action.description}</p>
                    {copyErrorActionId === action.id && (
                      <p className="text-warn mt-1 text-xs">
                        The clipboard API needs HTTPS or localhost. Select the command and copy
                        manually.
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default AIInsightPanel;
