import React, { useEffect, useState } from 'react';

import { RuntimeSettings, fetchRuntimeSettings, updateRuntimeSettings } from '../../services/api';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/card';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { Badge } from '../../components/ui/badge';
import { Skeleton } from '../../components/ui/skeleton';

interface FieldSpec {
  key: keyof RuntimeSettings;
  label: string;
  hint: string;
  min: number;
  max: number;
  unit: string;
}

const FIELDS: FieldSpec[] = [
  {
    key: 'poll_interval_seconds',
    label: 'Poll interval',
    hint: 'How often the machine is sampled. Lower means finer history and more load: one collection costs roughly 20ms plus a process scan that grows with how busy the host is.',
    min: 1,
    max: 3600,
    unit: 'seconds',
  },
  {
    key: 'metrics_retention_hours',
    label: 'Full resolution',
    hint: 'How long individual samples are kept before being rolled up. This is the expensive tier, at roughly 2.2 KB per sample.',
    min: 1,
    max: 24 * 30,
    unit: 'hours',
  },
  {
    key: 'minute_rollup_days',
    label: 'Per-minute history',
    hint: 'How long per-minute averages and peaks are kept.',
    min: 1,
    max: 3650,
    unit: 'days',
  },
  {
    key: 'five_minute_rollup_days',
    label: 'Per-five-minute history',
    hint: 'How long the coarse tier is kept. Data only leaves the system at the end of this window.',
    min: 1,
    max: 3650,
    unit: 'days',
  },
];

/**
 * Editable daemon settings.
 *
 * Only settings that genuinely take effect live appear here. A port or a
 * database path cannot change under a running process without reopening
 * sockets or re-deriving state, and offering them would be worse than saying
 * "restart required".
 */
export function RuntimeSettingsForm({ isAdmin }: { isAdmin: boolean }) {
  const [settings, setSettings] = useState<RuntimeSettings | null>(null);
  const [draft, setDraft] = useState<Partial<RuntimeSettings>>({});
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void fetchRuntimeSettings().then((s) => {
      if (!cancelled) setSettings(s);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!settings) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Collection</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </CardContent>
      </Card>
    );
  }

  const changed = Object.entries(draft).filter(
    ([k, v]) => v !== undefined && v !== settings[k as keyof RuntimeSettings],
  );

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      // Only the changed fields. The server treats absent keys as "leave
      // alone", so sending the whole object would let two admins editing
      // different settings overwrite each other.
      const patch = Object.fromEntries(changed) as Partial<RuntimeSettings>;
      const applied = await updateRuntimeSettings(patch);
      setSettings(applied);
      setDraft({});
      setSaved(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update settings');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Collection</CardTitle>
        <Badge variant="ok" className="ml-auto">
          applies without restart
        </Badge>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-4">
          {FIELDS.map((field) => {
            const value = draft[field.key] ?? settings[field.key];
            return (
              <div key={field.key} className="grid gap-1.5">
                <Label htmlFor={field.key}>
                  {field.label}
                  <span className="text-mute ml-2 font-normal">({field.unit})</span>
                </Label>
                <Input
                  id={field.key}
                  type="number"
                  inputMode="numeric"
                  min={field.min}
                  max={field.max}
                  value={String(value)}
                  disabled={!isAdmin || saving}
                  aria-describedby={`${field.key}-hint`}
                  onChange={(e) => setDraft((d) => ({ ...d, [field.key]: Number(e.target.value) }))}
                  className="max-w-40"
                />
                <p id={`${field.key}-hint`} className="text-mute text-xs leading-relaxed">
                  {field.hint}
                </p>
              </div>
            );
          })}

          {error && (
            <p
              role="alert"
              className="border-crit/40 bg-crit-soft text-crit rounded-md border px-3 py-2 text-xs"
            >
              {error}
            </p>
          )}
          {saved && !error && (
            <p role="status" className="text-ok text-xs">
              Applied. The collector is already using the new interval.
            </p>
          )}

          {isAdmin ? (
            <Button type="submit" disabled={saving || changed.length === 0}>
              {saving ? 'Applying…' : changed.length === 0 ? 'No changes' : 'Apply'}
            </Button>
          ) : (
            <p className="text-mute text-xs">
              Read-only for viewers. The poll interval controls the load this daemon places on the
              host, and retention controls how much history exists at all.
            </p>
          )}
        </form>
      </CardContent>
    </Card>
  );
}
