import React, { useCallback, useEffect, useState } from 'react';
import { CheckCheck, Copy, Loader2, MonitorSmartphone, Plus, ShieldOff } from 'lucide-react';

import {
  FleetAgent,
  IssuedJoinToken,
  JoinToken,
  createJoinToken,
  fetchAgents,
  fetchJoinTokens,
  revokeAgent,
} from '../../services/api';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { Skeleton } from '../../components/ui/skeleton';
import { LiveDot } from '../../components/ui/motion-primitives';

/** A device is considered live if it has reported within this window. */
const LIVE_WINDOW_MS = 90_000;

/** Renders a duration in the largest unit that keeps the number small. */
function humanDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.round(hours / 24)}d`;
}

function relativeTime(iso?: string): string {
  if (!iso) return 'never';
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return 'unknown';
  return `${humanDuration(Math.max(0, Math.round((Date.now() - then) / 1000)))} ago`;
}

/** How long until a future timestamp — the inverse of relativeTime. */
function timeUntil(iso: string): string {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return 'unknown';
  const seconds = Math.round((then - Date.now()) / 1000);
  if (seconds <= 0) return 'expired';
  return `expires in ${humanDuration(seconds)}`;
}

/**
 * Copy that degrades honestly.
 *
 * The clipboard API needs a secure context, and this console is routinely
 * served over plain HTTP on a LAN — so a copy button that silently does
 * nothing is a real outcome, not a hypothetical one.
 */
function CopyButton({ value, label }: { value: string; label: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle');

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setState('copied');
    } catch {
      setState('failed');
    }
    window.setTimeout(() => setState('idle'), 2400);
  };

  return (
    <Button type="button" variant="outline" size="sm" onClick={copy} aria-label={label}>
      {state === 'copied' ? (
        <>
          <CheckCheck className="size-3.5" aria-hidden="true" /> Copied
        </>
      ) : state === 'failed' ? (
        <>
          <Copy className="size-3.5" aria-hidden="true" /> Select it manually
        </>
      ) : (
        <>
          <Copy className="size-3.5" aria-hidden="true" /> Copy
        </>
      )}
    </Button>
  );
}

/** The one-time reveal of a freshly minted token. */
function IssuedToken({ issued }: { issued: IssuedJoinToken }) {
  return (
    <div className="border-brand/40 bg-brand/5 space-y-3 rounded-lg border p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-medium">
          Run this on <span className="text-brand">{issued.label || 'the new device'}</span>
        </p>
        <Badge variant="outline" className="text-2xs">
          {timeUntil(issued.expires_at)}
        </Badge>
      </div>

      <pre className="bg-panel border-line overflow-x-auto rounded-md border p-3 text-xs leading-relaxed">
        <code>{issued.command}</code>
      </pre>

      <div className="flex flex-wrap items-center gap-2">
        <CopyButton value={issued.command} label="Copy the enrolment command" />
        {/* Stated plainly, because there is no way to recover it afterwards. */}
        <p className="text-mute text-2xs">
          This token is shown once and cannot be retrieved later. It can be used by one device.
        </p>
      </div>
    </div>
  );
}

function DeviceRow({ agent, onRevoke }: { agent: FleetAgent; onRevoke: (a: FleetAgent) => void }) {
  const revoked = Boolean(agent.revoked_at);
  const lastSeen = agent.last_seen_at ? new Date(agent.last_seen_at).getTime() : 0;
  const live = !revoked && Date.now() - lastSeen < LIVE_WINDOW_MS;

  return (
    <div className="border-line flex flex-wrap items-center justify-between gap-3 border-b py-3 last:border-b-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          {live ? <LiveDot tone="ok" /> : <span className="bg-mute/50 size-2 rounded-full" />}
          <span className="truncate text-sm font-medium">
            {agent.label || agent.hostname || 'unnamed device'}
          </span>
          {revoked && (
            <Badge variant="outline" className="text-2xs border-crit/40 text-crit">
              revoked
            </Badge>
          )}
        </div>
        <p className="text-mute text-2xs mt-1 truncate">
          {agent.hostname || 'unknown host'} · {agent.agent_version || 'version unknown'} · last
          seen {relativeTime(agent.last_seen_at)}
        </p>
      </div>

      {!revoked && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => onRevoke(agent)}
          className="text-crit hover:bg-crit/10"
        >
          <ShieldOff className="size-3.5" aria-hidden="true" /> Revoke
        </Button>
      )}
    </div>
  );
}

/**
 * Devices — enrol a machine, watch it appear, and withdraw its access.
 *
 * Before this screen the only way to add a host was to copy one shared key
 * into every machine's config by hand, which meant no per-machine identity and
 * no way to remove one host without re-keying the entire fleet.
 */
export function DevicesPanel({ isAdmin }: { isAdmin: boolean }) {
  const [agents, setAgents] = useState<FleetAgent[] | null>(null);
  const [pending, setPending] = useState<JoinToken[]>([]);
  const [issued, setIssued] = useState<IssuedJoinToken | null>(null);
  const [label, setLabel] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const list = await fetchAgents();
      setAgents(list);
      if (isAdmin) setPending(await fetchJoinTokens());
      setError(null);
    } catch (err) {
      setAgents([]);
      setError(err instanceof Error ? err.message : 'Failed to load devices');
    }
  }, [isAdmin]);

  useEffect(() => {
    void load();
    // Poll rather than push: enrolment is rare, and a device appearing within
    // a few seconds of running the command is responsive enough.
    const timer = window.setInterval(() => void load(), 10_000);
    return () => window.clearInterval(timer);
  }, [load]);

  const addDevice = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      setIssued(await createJoinToken(label.trim()));
      setLabel('');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create a join token');
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (agent: FleetAgent) => {
    const name = agent.label || agent.hostname || 'this device';
    // Revocation is immediate and cannot be undone — the machine must re-enrol
    // with a new token — so it is worth one confirmation.
    if (!window.confirm(`Revoke ${name}? It will stop being able to send data.`)) return;
    try {
      await revokeAgent(agent.id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to revoke this device');
    }
  };

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      {isAdmin && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Plus className="size-4" aria-hidden="true" /> Add a device
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-mute text-sm">
              Install SysSentient on the machine, then run the command below on it. The device
              appears here once it reports.
            </p>

            <form onSubmit={addDevice} className="flex flex-wrap items-end gap-2">
              <div className="min-w-[12rem] flex-1 space-y-1.5">
                <Label htmlFor="device-label">Name</Label>
                <Input
                  id="device-label"
                  value={label}
                  onChange={(e) => setLabel(e.target.value)}
                  placeholder="web-01"
                  maxLength={64}
                />
              </div>
              <Button type="submit" disabled={busy}>
                {busy ? (
                  <>
                    <Loader2 className="size-4 animate-spin" aria-hidden="true" /> Creating
                  </>
                ) : (
                  'Generate command'
                )}
              </Button>
            </form>

            {issued && <IssuedToken issued={issued} />}

            {pending.length > 0 && (
              <div className="text-mute text-2xs">
                {pending.length} invitation{pending.length === 1 ? '' : 's'} still unused
                {pending.length > 0 && `: ${pending.map((t) => t.label || 'unnamed').join(', ')}`}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <MonitorSmartphone className="size-4" aria-hidden="true" /> Enrolled devices
          </CardTitle>
        </CardHeader>
        <CardContent>
          {error && (
            <p role="alert" className="text-crit mb-3 text-sm">
              {error}
            </p>
          )}

          {agents === null ? (
            <div className="space-y-3">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : agents.length === 0 ? (
            <p className="text-mute text-sm">
              No devices enrolled yet. This server is either running all-in-one, or waiting for its
              first agent.
            </p>
          ) : (
            <div>
              {agents.map((agent) => (
                <DeviceRow key={agent.id} agent={agent} onRevoke={revoke} />
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
