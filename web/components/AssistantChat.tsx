import { useRef, useState } from 'react';
import { CornerDownLeft, Loader2, Sparkles, Wrench } from 'lucide-react';

import { askAssistant, type ChatMessage } from '../services/api';
import { Card, CardContent } from './ui/card';
import { SectionHeading } from './ui/section-heading';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { cn } from '../lib/utils';

/** Questions worth one click, chosen to show what the tools can reach. */
const SUGGESTIONS = [
  'What happened on this host in the last hour?',
  'CPU spiked recently — which process caused it?',
  'Which host is under the most memory pressure?',
];

interface Turn extends ChatMessage {
  tools?: string[];
}

/**
 * Ask questions about the fleet.
 *
 * Distinct from the one-shot analysis beside it: the assistant can look things
 * up, then look up more based on what it found, rather than reasoning over a
 * single snapshot it was handed. The tools it consulted are shown with every
 * answer, so a claim can be checked against the data it came from instead of
 * being taken on trust.
 */
export function AssistantChat() {
  const [turns, setTurns] = useState<Turn[]>([]);
  const [question, setQuestion] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const scroller = useRef<HTMLDivElement>(null);

  const ask = async (text: string) => {
    const trimmed = text.trim();
    if (!trimmed || busy) return;

    setError(null);
    setBusy(true);
    setQuestion('');
    const history = turns.map(({ role, text: t }) => ({ role, text: t }));
    setTurns((prev) => [...prev, { role: 'user', text: trimmed }]);

    try {
      const reply = await askAssistant(trimmed, history);
      setTurns((prev) => [...prev, { role: 'model', text: reply.text, tools: reply.tool_calls }]);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'The assistant failed');
    } finally {
      setBusy(false);
      // Let the new turn render before scrolling to it.
      window.setTimeout(
        () => scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: 'smooth' }),
        50,
      );
    }
  };

  return (
    <Card>
      <CardContent className="p-5">
        <SectionHeading
          eyebrow="Ask"
          title="Assistant"
          action={
            <span className="text-mute font-mono text-2xs">reads your data, changes nothing</span>
          }
        />

        <div ref={scroller} className="mt-4 max-h-[26rem] space-y-3 overflow-y-auto">
          {turns.length === 0 && (
            <div className="space-y-2">
              <p className="text-mute text-sm">
                Ask about any machine reporting here. The assistant queries real metrics,
                processes, logs and alerts to answer — it cannot make changes.
              </p>
              <div className="flex flex-wrap gap-1.5">
                {SUGGESTIONS.map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => void ask(s)}
                    className="border-line text-mute hover:text-fg hover:border-brand/40 rounded-full border px-2.5 py-1 text-2xs transition-colors"
                  >
                    {s}
                  </button>
                ))}
              </div>
            </div>
          )}

          {turns.map((turn, i) => (
            <div
              key={i}
              className={cn(
                'rounded-lg px-3 py-2 text-sm',
                turn.role === 'user'
                  ? 'bg-panel-strong/60 ml-8'
                  : 'border-line border',
              )}
            >
              {turn.role === 'model' && (
                <Sparkles className="text-accent mb-1 size-3.5" aria-hidden="true" />
              )}
              <p className="whitespace-pre-wrap leading-relaxed">{turn.text}</p>

              {/* The evidence trail. An answer you cannot check is a guess with
                  better formatting. */}
              {turn.tools && turn.tools.length > 0 && (
                <p className="text-mute mt-2 flex flex-wrap items-center gap-1 font-mono text-2xs">
                  <Wrench className="size-3" aria-hidden="true" />
                  {turn.tools.join(' → ')}
                </p>
              )}
            </div>
          ))}

          {busy && (
            <div className="text-mute flex items-center gap-2 px-3 text-sm">
              <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
              consulting the data…
            </div>
          )}
        </div>

        {error && (
          <p role="alert" className="text-crit mt-3 text-sm">
            {error}
          </p>
        )}

        <form
          className="mt-4 flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void ask(question);
          }}
        >
          <Input
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            placeholder="Ask about a host, a spike, an alert…"
            aria-label="Ask the assistant"
            disabled={busy}
          />
          <Button type="submit" disabled={busy || !question.trim()}>
            <CornerDownLeft className="size-3.5" aria-hidden="true" />
            Ask
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
