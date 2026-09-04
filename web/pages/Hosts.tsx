import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Server, ArrowRight, CircleAlert } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

import { useDashboard } from '../hooks/useDashboardData';
import { fetchAgents, type FleetAgent } from '../services/api';
import { Card, CardContent } from '../components/ui/card';
import { ScreenHeading, SectionHeading } from '../components/ui/section-heading';
import { Badge } from '../components/ui/badge';
import { LiveDot } from '../components/ui/motion-primitives';
import { cn } from '../lib/utils';

/** A host is live if it reported within this window. */
const LIVE_WINDOW_MS = 90_000;

function ago(iso?: string): string {
  if (!iso) return 'never';
  const then = Date.parse(iso);
  if (!Number.isFinite(then)) return 'unknown';
  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

/**
 * Every machine reporting to this server.
 *
 * "Host" meant three unreconciled things — a hostname on a sample, a row in
 * /api/hosts, and an enrolled agent — with no page listing any of them. The
 * switcher stayed hidden until a second host existed, so the concept and the
 * way to add another were both unmarked.
 */
const Hosts: React.FC = () => {
  const { hosts, selectHost, selectedHost, current } = useDashboard();
  const navigate = useNavigate();
  const [agents, setAgents] = useState<FleetAgent[]>([]);

  const load = useCallback(async () => {
    try {
      setAgents(await fetchAgents());
    } catch {
      // A viewer without permission still gets the host list; the agent
      // credentials are simply not shown.
      setAgents([]);
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15000);
    return () => window.clearInterval(timer);
  }, [load]);

  // One row per machine, joining the two views of a host by id.
  const rows = useMemo(
    () =>
      hosts.map((host) => {
        const agent = agents.find((a) => a.host_id === host.hostId);
        const lastSeen = Date.parse(host.lastSeen);
        return {
          ...host,
          live: Number.isFinite(lastSeen) && Date.now() - lastSeen < LIVE_WINDOW_MS,
          label: agent?.label ?? '',
          revoked: Boolean(agent?.revoked_at),
          enrolled: Boolean(agent),
        };
      }),
    [hosts, agents],
  );

  const openHost = (hostId: string) => {
    selectHost(hostId);
    navigate('/');
  };

  return (
    <>
      <ScreenHeading
        eyebrow="Fleet"
        title="Hosts"
        description="Every machine reporting to this server. Selecting one scopes the whole console to it."
      />

      <Card>
        <CardContent className="p-5">
          <SectionHeading
            eyebrow="Reporting"
            title="Machines"
            action={
              <span className="text-mute tabular font-mono text-2xs">{rows.length}</span>
            }
          />

          {rows.length === 0 ? (
            <div className="text-mute mt-6 space-y-2 text-sm">
              <p>No machine has reported yet.</p>
              <p className="text-2xs">
                A single-node install registers itself on its first sample. To add another
                machine, open Settings → Devices.
              </p>
            </div>
          ) : (
            <ul className="mt-4 divide-y divide-line">
              {rows.map((host) => (
                <li key={host.hostId}>
                  <button
                    type="button"
                    onClick={() => openHost(host.hostId)}
                    className={cn(
                      'flex w-full items-center gap-3 py-3 text-left transition-colors',
                      selectedHost === host.hostId ? 'text-brand' : 'hover:text-brand',
                    )}
                  >
                    {host.live ? (
                      <LiveDot tone="ok" />
                    ) : (
                      <span className="bg-mute/40 size-2 rounded-full" aria-hidden="true" />
                    )}

                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate text-sm font-medium">
                          {host.label || host.hostname || 'unnamed host'}
                        </span>
                        {host.label && host.hostname && host.label !== host.hostname && (
                          <span className="text-mute font-mono text-2xs">{host.hostname}</span>
                        )}
                        {host.revoked && (
                          <Badge variant="outline" className="text-2xs border-crit/40 text-crit">
                            revoked
                          </Badge>
                        )}
                        {!host.enrolled && (
                          <Badge variant="outline" className="text-2xs">
                            local
                          </Badge>
                        )}
                        {host.hostId === current.hostId && (
                          <Badge variant="secondary" className="text-2xs">
                            this machine
                          </Badge>
                        )}
                      </div>
                      {/* The id is what everything else keys on, so it is
                          worth showing rather than hiding behind a hostname
                          that two machines can share. */}
                      <p className="text-mute mt-0.5 truncate font-mono text-2xs">
                        {host.hostId.slice(0, 16)} · {host.agentVersion || 'version unknown'} ·
                        last seen {ago(host.lastSeen)}
                      </p>
                    </div>

                    <ArrowRight className="text-mute size-4 shrink-0" aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent className="text-mute space-y-2 p-5 text-2xs leading-relaxed">
          <div className="flex items-start gap-2">
            <Server className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
            <p>
              A host is identified by a stable id derived from the machine, not by its
              hostname — two machines can share a name, and a renamed machine keeps its
              history.
            </p>
          </div>
          <div className="flex items-start gap-2">
            <CircleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
            <p>
              <span className="text-fg">local</span> marks a machine that reports without an
              enrolled credential — the all-in-one install collecting from itself, or an
              agent using the shared fleet key.
            </p>
          </div>
        </CardContent>
      </Card>
    </>
  );
};

export default Hosts;
