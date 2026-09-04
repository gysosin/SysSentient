import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { fetchAlertHistory } from '../services/api';
import type { AlertEvent } from '../types';

/** One thing worth telling the operator about. */
export interface Notification {
  id: string;
  at: number;
  severity: string;
  state: string;
  title: string;
  detail: string;
  read: boolean;
}

const SEEN_KEY = 'syssentient.notifications.seen';
const POLL_MS = 15000;

/** Reads the high-water mark of what the operator has already seen. */
function loadSeenAt(): number {
  try {
    const raw = window.localStorage.getItem(SEEN_KEY);
    const parsed = raw ? Number.parseInt(raw, 10) : 0;
    return Number.isFinite(parsed) ? parsed : 0;
  } catch {
    // Private windows and blocked site data throw rather than returning null.
    return 0;
  }
}

function storeSeenAt(at: number) {
  try {
    window.localStorage.setItem(SEEN_KEY, String(at));
  } catch {
    // Unread state is a convenience; failing to persist it must not break
    // the panel.
  }
}

/**
 * The notification feed.
 *
 * The console had no notification surface at all — no bell, no toasts, no
 * unread state. Alerts existed only on their own page and as a count badge, so
 * anything that happened while you were looking elsewhere was invisible until
 * you went looking for it.
 */
export function useNotifications() {
  const [events, setEvents] = useState<AlertEvent[]>([]);
  const [seenAt, setSeenAt] = useState<number>(() => loadSeenAt());
  const known = useRef<Set<string>>(new Set());
  const [fresh, setFresh] = useState<Notification[]>([]);

  const load = useCallback(async () => {
    const history = await fetchAlertHistory(50);
    setEvents(history);
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [load]);

  const notifications = useMemo<Notification[]>(
    () =>
      events.map((e) => {
        const at = Date.parse(e.occurredAt);
        return {
          id: `${e.ruleId}-${e.hostname}-${e.occurredAt}`,
          at: Number.isFinite(at) ? at : 0,
          severity: e.severity,
          state: e.state,
          title: e.ruleName,
          detail: `${e.metric} = ${e.value.toFixed(1)} (threshold ${e.threshold}) · ${e.hostname}`,
          read: Number.isFinite(at) ? at <= seenAt : true,
        };
      }),
    [events, seenAt],
  );

  const unread = notifications.filter((n) => !n.read).length;

  // Anything arriving after the first load is surfaced as a toast, so a
  // transition that happens while you are on another screen is not silent.
  useEffect(() => {
    const first = known.current.size === 0;
    const arrivals = notifications.filter((n) => !known.current.has(n.id));
    arrivals.forEach((n) => known.current.add(n.id));
    if (first || arrivals.length === 0) return;
    setFresh((prev) => [...arrivals.filter((n) => !n.read), ...prev].slice(0, 3));
  }, [notifications]);

  const markAllRead = useCallback(() => {
    const newest = notifications.reduce((max, n) => Math.max(max, n.at), 0);
    setSeenAt(newest);
    storeSeenAt(newest);
  }, [notifications]);

  const dismissToast = useCallback((id: string) => {
    setFresh((prev) => prev.filter((n) => n.id !== id));
  }, []);

  return { notifications, unread, markAllRead, toasts: fresh, dismissToast, refresh: load };
}
