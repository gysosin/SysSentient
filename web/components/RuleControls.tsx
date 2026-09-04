import { useState } from 'react';
import { BellOff, RotateCcw } from 'lucide-react';

import { resetAlertRule, updateAlertRule, type AlertRuleView } from '../services/api';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { cn } from '../lib/utils';

/** Mute durations an operator actually reaches for. */
const MUTE_CHOICES = [
  { hours: 1, label: '1h' },
  { hours: 8, label: '8h' },
  { hours: 24, label: '24h' },
];

/**
 * Edit, disable, mute or reset one alert rule.
 *
 * Rules were hardcoded and read-only. `Rule.Enabled` was honoured by the
 * evaluator but nothing could ever set it false, and a threshold could not be
 * tuned without editing the source and rebuilding.
 */
export function RuleControls({
  rule,
  isAdmin,
  onChange,
}: {
  rule: AlertRuleView;
  isAdmin: boolean;
  onChange: (rules: AlertRuleView[]) => void;
}) {
  const [threshold, setThreshold] = useState(String(rule.threshold));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const muted = rule.muted_until ? new Date(rule.muted_until) > new Date() : false;

  const apply = async (fn: () => Promise<AlertRuleView[]>) => {
    setBusy(true);
    setError(null);
    try {
      onChange(await fn());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Change failed');
    } finally {
      setBusy(false);
    }
  };

  if (!isAdmin) {
    return (
      <span className="text-mute text-2xs">
        {muted ? 'muted' : rule.enabled ? '' : 'disabled'}
      </span>
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Input
        aria-label={`${rule.name} threshold`}
        value={threshold}
        onChange={(e) => setThreshold(e.target.value)}
        onBlur={() => {
          const parsed = Number.parseFloat(threshold);
          if (!Number.isFinite(parsed) || parsed === rule.threshold) {
            setThreshold(String(rule.threshold));
            return;
          }
          void apply(() => updateAlertRule(rule.id, { threshold: parsed }));
        }}
        className="h-7 w-20 text-2xs"
        disabled={busy}
      />

      <Button
        variant="outline"
        size="sm"
        disabled={busy}
        onClick={() => void apply(() => updateAlertRule(rule.id, { enabled: !rule.enabled }))}
      >
        {rule.enabled ? 'Disable' : 'Enable'}
      </Button>

      {/* Mute stops the paging without stopping the evaluation, so the alert
          still shows on the dashboard while it is silenced. */}
      {muted ? (
        <Button
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => void apply(() => updateAlertRule(rule.id, { mute_hours: 0 }))}
          className="text-warn"
        >
          <BellOff className="size-3" aria-hidden="true" /> Unmute
        </Button>
      ) : (
        MUTE_CHOICES.map((choice) => (
          <Button
            key={choice.hours}
            variant="ghost"
            size="sm"
            disabled={busy}
            title={`Silence notifications for ${choice.label}`}
            onClick={() => void apply(() => updateAlertRule(rule.id, { mute_hours: choice.hours }))}
            className="text-mute px-1.5"
          >
            {choice.label}
          </Button>
        ))
      )}

      {rule.overridden && (
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          title="Restore the built-in default"
          onClick={() => void apply(() => resetAlertRule(rule.id))}
          className="text-mute"
        >
          <RotateCcw className="size-3" aria-hidden="true" />
        </Button>
      )}

      {error && (
        <span role="alert" className={cn('text-crit text-2xs')}>
          {error}
        </span>
      )}
    </div>
  );
}
