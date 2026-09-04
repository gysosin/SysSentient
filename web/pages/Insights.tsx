import React from 'react';
import { Check, LockKeyhole, ShieldAlert } from 'lucide-react';

import { useDashboard } from '../hooks/useDashboardData';
import AIInsightPanel from '../components/AIInsightPanel';
import { Card, CardContent } from '../components/ui/card';
import { ScreenHeading, SectionHeading } from '../components/ui/section-heading';
import { cn } from '../lib/utils';

/** What the daemon strips before anything leaves the machine. */
const REDACTED = [
  'CPU, memory, disk, network and process aggregates are included.',
  'The hostname is retained so a fleet-wide answer can be correlated.',
  'IP and e-mail addresses and home-directory usernames are removed from logs and process names.',
];

/** Relative age, so a timeline reads without arithmetic. */
function ago(timestamp: number): string {
  if (!timestamp) return 'unknown';
  const seconds = Math.max(0, Math.round((Date.now() - timestamp) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

const STATUS_TONE: Record<string, string> = {
  Healthy: 'text-ok',
  Warning: 'text-warn',
  Critical: 'text-crit',
};

const Insights: React.FC = () => {
  const { ai } = useDashboard();
  const [selected, setSelected] = React.useState<number | null>(null);

  // The newest analysis unless one is pinned from the timeline.
  const shown = React.useMemo(() => {
    if (selected === null) return ai.result;
    return ai.history.find((entry) => entry.id === selected)?.analysis ?? ai.result;
  }, [selected, ai.history, ai.result]);

  return (
    <>
      <ScreenHeading
        eyebrow="Accountable assistance"
        title="AI insights"
        description="A diagnosis is only useful when its evidence, its boundaries and its suggested actions are all visible."
      />

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1.3fr)_minmax(280px,0.7fr)]">
        <div className="space-y-4">
          <AIInsightPanel
            analysis={shown}
            error={ai.error}
            loading={ai.loading}
            onRefresh={ai.run}
          />

          {/* Every stored analysis. Previously the dashboard read only the
              newest row and discarded the rest, and loaded even that one only
              when the WebSocket was down — so a healthy connection meant the
              page reported "No analysis yet" against a full database. */}
          <Card>
            <CardContent className="p-5">
              <SectionHeading
                eyebrow="History"
                title="Past analyses"
                action={
                  <span className="text-mute tabular font-mono text-2xs">
                    {ai.history.length} stored
                  </span>
                }
              />

              {ai.history.length === 0 ? (
                <p className="text-mute mt-4 text-sm">
                  Nothing stored yet. Analyses are kept as they are produced, on demand or
                  automatically when a threshold is crossed.
                </p>
              ) : (
                <ul className="mt-4 divide-y divide-line">
                  {ai.history.map((entry) => {
                    const active = entry.id === selected || (selected === null && entry === ai.history[0]);
                    return (
                      <li key={entry.id}>
                        <button
                          type="button"
                          onClick={() => setSelected(entry.id)}
                          aria-current={active ? 'true' : undefined}
                          className={cn(
                            'flex w-full items-center gap-3 py-3 text-left transition-colors',
                            active ? 'text-fg' : 'text-mute hover:text-fg',
                          )}
                        >
                          <span
                            className={cn(
                              'shrink-0 text-2xs font-mono uppercase tracking-wide',
                              STATUS_TONE[entry.status] ?? 'text-mute',
                            )}
                          >
                            {entry.status || 'unknown'}
                          </span>
                          <span className="min-w-0 flex-1 truncate text-sm">
                            {entry.analysis.summary}
                          </span>
                          <span className="text-mute tabular shrink-0 font-mono text-2xs">
                            {ago(entry.timestamp)}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardContent className="p-5">
            <SectionHeading eyebrow="Data boundary" title="What left the host" />

            {/* Stated as a list of facts rather than a paragraph of policy.
                Anyone deciding whether to enable this feature is auditing it,
                and an audit is a checklist. */}
            <ul className="text-muted-foreground mt-5 space-y-4 text-2xs leading-relaxed">
              {REDACTED.map((item) => (
                <li key={item} className="flex gap-3">
                  <Check className="text-ok mt-0.5 size-4 shrink-0" aria-hidden="true" />
                  <span>{item}</span>
                </li>
              ))}
            </ul>

            <div className="border-warn/30 bg-warn-soft mt-6 flex gap-2.5 rounded-lg border p-3">
              <ShieldAlert className="text-warn mt-0.5 size-4 shrink-0" aria-hidden="true" />
              <p className="text-warn text-xs leading-relaxed">
                Recommended commands are generated by a language model and are not validated by the
                daemon. Read them before running anything, and never paste one into a root shell
                unexamined.
              </p>
            </div>

            <div className="border-line bg-panel-strong text-mute mt-4 rounded-md border p-3 text-2xs leading-relaxed">
              <LockKeyhole className="text-brand mb-2 size-4" aria-hidden="true" />
              Analysis runs against a single current sample plus recent logs. It is triggered
              automatically when CPU or memory crosses a threshold, and manually from this page.
              Gemini access is configured by the administrator and is off until a key is set.
            </div>
          </CardContent>
        </Card>
      </div>
    </>
  );
};

export default Insights;
