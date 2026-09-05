import React, { useCallback, useEffect, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { AlertTriangle, BellOff, CheckCircle2, Clock } from 'lucide-react';

import { Alert, AlertEvent, AlertRule } from '../types';
import {
  acknowledgeAlert,
  fetchActiveAlerts,
  fetchAlertHistory,
  fetchAlertRules,
} from '../services/api';
import { RuleControls } from '../components/RuleControls';
import { fetchAlertRuleViews, type AlertRuleView } from '../services/api';
import { useAuth } from '../hooks/useAuth';
import { useDashboard } from '../hooks/useDashboardData';
import { usePageVisible } from '../hooks/usePageVisible';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/card';
import { ScreenHeading } from '../components/ui/section-heading';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Skeleton } from '../components/ui/skeleton';
import { cn } from '../lib/utils';

const ALERT_REFRESH_MS = 5000;

const relative = (iso: string): string => {
  const ms = Date.parse(iso);
  if (!Number.isFinite(ms)) return '';
  const seconds = Math.max(0, Math.round((Date.now() - ms) / 1000));
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
};

const formatWhen = (iso: string): string => {
  const ms = Date.parse(iso);
  return Number.isFinite(ms) ? new Date(ms).toLocaleString() : '—';
};

const Alerts: React.FC = () => {
  const { selectedHost } = useDashboard();
  const { user } = useAuth();
  const visible = usePageVisible();
  const isAdmin = user?.role === 'admin';
  // The rule views carry override and mute state, which the plain rule list
  // does not.
  const [ruleViews, setRuleViews] = useState<AlertRuleView[]>([]);
  const [active, setActive] = useState<Alert[]>([]);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [history, setHistory] = useState<AlertEvent[]>([]);
  const [ackError, setAckError] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  const refresh = useCallback(async () => {
    const [a, r, h, v] = await Promise.all([
      fetchActiveAlerts(selectedHost),
      fetchAlertRules(),
      fetchAlertHistory(50),
      fetchAlertRuleViews(),
    ]);
    setActive(a);
    setRules(r);
    setHistory(h);
    setRuleViews(v);
    setLoaded(true);
  }, [selectedHost]);

  useEffect(() => {
    if (!visible) return;
    refresh();
    const id = setInterval(refresh, ALERT_REFRESH_MS);
    return () => clearInterval(id);
  }, [refresh, visible]);

  const handleAcknowledge = async (alert: Alert) => {
    setAckError(null);
    try {
      await acknowledgeAlert(alert.ruleId, alert.hostname ? selectedHost : '');
      await refresh();
    } catch (error) {
      setAckError(error instanceof Error ? error.message : 'Failed to acknowledge');
    }
  };

  const firing = active.filter((a) => a.state === 'firing');
  const pending = active.filter((a) => a.state === 'pending');

  return (
    <div className="space-y-4">
      <ScreenHeading
        eyebrow="Rules and lifecycle"
        title="Alerts"
        description="What is firing now, the policy that decides it, and every transition that led here."
      />

      {ackError && (
        <div className="border-crit/40 bg-crit-soft text-crit rounded-lg border px-4 py-2.5 text-sm" role="alert">
          {ackError}
        </div>
      )}

      <Card className={cn(firing.length > 0 && 'border-crit/50 ring-crit/20 ring-1')}>
        <CardHeader>
          <AlertTriangle className={cn('size-4', firing.length > 0 ? 'text-crit' : 'text-mute')} />
          <CardTitle>Firing</CardTitle>
          <Badge variant={firing.length > 0 ? 'crit' : 'secondary'} className="tabular">
            {firing.length}
          </Badge>
        </CardHeader>
        <CardContent>
          {!loaded ? (
            <div className="space-y-2">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-16 w-full" />
            </div>
          ) : firing.length === 0 ? (
            <div className="py-10 text-center">
              <CheckCircle2 className="text-ok mx-auto mb-2 size-6" />
              <p className="text-sm">Nothing firing</p>
              <p className="text-mute mt-1 text-xs">
                Rules are evaluated on every sample.
              </p>
            </div>
          ) : (
            <ul className="space-y-2">
              <AnimatePresence initial={false}>
                {firing.map((alert) => (
                  <motion.li
                    key={`${alert.hostname}-${alert.ruleId}`}
                    layout
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, height: 0 }}
                    className={cn(
                      'flex flex-wrap items-center gap-3 rounded-lg border p-3.5',
                      alert.severity === 'critical'
                        ? 'border-crit/35 bg-crit-soft'
                        : 'border-warn/35 bg-warn-soft',
                    )}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <p className="font-medium">{alert.ruleName}</p>
                        <Badge variant={alert.severity === 'critical' ? 'crit' : 'warn'}>
                          {alert.severity}
                        </Badge>
                        {alert.acknowledged && (
                          <Badge variant="outline" className="gap-1">
                            <BellOff className="size-3" />
                            acknowledged
                          </Badge>
                        )}
                      </div>
                      <p className="text-mute tabular mt-1 text-xs">
                        {alert.metric} = {alert.value.toFixed(1)} (threshold {alert.threshold})
                        {alert.hostname && ` · ${alert.hostname}`}
                        {alert.startedAt && ` · firing ${relative(alert.startedAt)}`}
                      </p>
                    </div>
                    {!alert.acknowledged && (
                      <Button variant="outline" size="sm" onClick={() => handleAcknowledge(alert)}>
                        Acknowledge
                      </Button>
                    )}
                  </motion.li>
                ))}
              </AnimatePresence>
            </ul>
          )}
        </CardContent>
      </Card>

      {pending.length > 0 && (
        <Card>
          <CardHeader>
            <Clock className="text-warn size-4" />
            <CardTitle>Pending</CardTitle>
            <Badge variant="warn" className="tabular">{pending.length}</Badge>
          </CardHeader>
          <CardContent>
            <p className="text-mute mb-3 text-xs">
              Condition is true but has not held long enough to fire. This is what suppresses
              transient spikes.
            </p>
            <ul className="space-y-2">
              {pending.map((alert) => (
                <li key={`${alert.hostname}-${alert.ruleId}`} className="rounded-lg border p-3">
                  <p className="text-sm font-medium">{alert.ruleName}</p>
                  <p className="text-mute tabular mt-0.5 text-xs">
                    {alert.metric} = {alert.value.toFixed(1)} · waiting {relative(alert.startedAt)}
                  </p>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Rules</CardTitle>
          <Badge variant="secondary" className="tabular">{rules.length}</Badge>
        </CardHeader>
        <CardContent className="px-0">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[560px] text-sm">
              <thead>
                <tr className="text-mute border-line border-b font-mono text-2xs tracking-[0.15em] uppercase">
                  <th scope="col" className="px-5 py-2.5 text-left font-medium">Rule</th>
                  <th scope="col" className="px-5 py-2.5 text-left font-medium">Condition</th>
                  <th scope="col" className="px-5 py-2.5 text-left font-medium">For</th>
                  <th scope="col" className="px-5 py-2.5 text-left font-medium">Severity</th>
                  <th scope="col" className="px-5 py-2.5 text-left font-medium">Control</th>
                </tr>
              </thead>
              <tbody>
                {rules.map((rule) => (
                  <tr key={rule.id} className="hover:bg-panel-strong/60 border-b transition-colors last:border-0">
                    <td className="px-5 py-2.5 font-medium">
                      {rule.name}
                      {!rule.enabled && (
                        <span className="text-mute ml-2 text-xs">(disabled)</span>
                      )}
                    </td>
                    <td className="text-mute tabular px-5 py-2.5 font-mono text-xs">
                      {rule.metric} {rule.op} {rule.threshold}
                    </td>
                    <td className="text-mute tabular px-5 py-2.5 text-xs">
                      {rule.forLabel}
                    </td>
                    <td className="px-5 py-2.5">
                      <Badge variant={rule.severity === 'critical' ? 'crit' : 'warn'}>
                        {rule.severity}
                      </Badge>
                    </td>
                    <td className="px-5 py-2.5">
                      {ruleViews.find((v) => v.id === rule.id) && (
                        <RuleControls
                          rule={ruleViews.find((v) => v.id === rule.id)!}
                          isAdmin={isAdmin}
                          onChange={setRuleViews}
                        />
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="text-mute px-5 pt-3 text-xs">
            Thresholds, enablement and mutes are stored on the server and survive a restart.
            A muted rule still evaluates and still appears above — it just stops notifying.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>History</CardTitle>
          <Badge variant="secondary" className="tabular">{history.length}</Badge>
        </CardHeader>
        <CardContent>
          {history.length === 0 ? (
            <div className="py-10 text-center">
              <p className="text-sm">No alert transitions recorded yet</p>
              <p className="text-mute mt-1 text-xs">
                Only fired and resolved events are stored, not every poll.
              </p>
            </div>
          ) : (
            <ul className="max-h-[50vh] space-y-0.5 overflow-y-auto font-mono text-2xs">
              {history.map((event, idx) => (
                <li
                  key={`${event.ruleId}-${event.occurredAt}-${idx}`}
                  className="hover:bg-panel-strong/60 grid grid-cols-[150px_80px_1fr] gap-3 rounded px-2 py-1 transition-colors"
                >
                  <span className="text-mute tabular">{formatWhen(event.occurredAt)}</span>
                  <span
                    className={cn(
                      'font-semibold uppercase',
                      event.state === 'firing' ? 'text-crit' : event.state === 'resolved' ? 'text-ok' : 'text-warn',
                    )}
                  >
                    {event.state}
                  </span>
                  <span>
                    {event.ruleName} — {event.metric} = {event.value.toFixed(1)} (threshold{' '}
                    {event.threshold})
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default Alerts;
