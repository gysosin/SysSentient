import { useEffect, useRef, useState } from 'react';
import { Bell, BellOff, Check, X } from 'lucide-react';

import { useNotifications, type Notification } from '../hooks/useNotifications';
import { cn } from '../lib/utils';

const SEVERITY_TONE: Record<string, string> = {
  critical: 'text-crit',
  warning: 'text-warn',
  info: 'text-mute',
};

function ago(at: number): string {
  if (!at) return '';
  const seconds = Math.max(0, Math.round((Date.now() - at) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.round(hours / 24)}d`;
}

function Row({ item }: { item: Notification }) {
  return (
    <li className="border-line flex gap-3 border-b px-4 py-3 last:border-b-0">
      <span
        className={cn(
          'mt-1.5 size-1.5 shrink-0 rounded-full',
          item.read ? 'bg-mute/40' : 'bg-brand',
        )}
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-2">
          <span className={cn('truncate text-sm font-medium', SEVERITY_TONE[item.severity])}>
            {item.title}
          </span>
          <span className="text-mute tabular shrink-0 font-mono text-2xs">{ago(item.at)}</span>
        </div>
        <p className="text-mute mt-0.5 truncate font-mono text-2xs">{item.detail}</p>
        <span className="text-mute text-2xs uppercase tracking-wide">{item.state}</span>
      </div>
    </li>
  );
}

/**
 * The bell, its unread count, and the panel behind it.
 *
 * Before this the console had no notification surface at all: no bell, no
 * toasts, no unread state. Alerts lived on their own page and as a count on a
 * nav link, so anything that happened while you were looking elsewhere was
 * invisible until you went looking for it.
 */
export function NotificationCenter() {
  const { notifications, unread, markAllRead } = useNotifications();
  const [open, setOpen] = useState(false);
  const panel = useRef<HTMLDivElement>(null);

  // Close on an outside click or Escape, like every other transient surface.
  useEffect(() => {
    if (!open) return;
    const onPointer = (e: MouseEvent) => {
      if (panel.current && !panel.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    document.addEventListener('mousedown', onPointer);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointer);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  return (
    <div className="relative" ref={panel}>
      <button
        type="button"
        onClick={() => {
          setOpen((v) => !v);
          if (!open) markAllRead();
        }}
        aria-label={unread > 0 ? `Notifications, ${unread} unread` : 'Notifications'}
        aria-expanded={open}
        className="text-mute hover:text-fg relative rounded-md p-1.5 transition-colors"
      >
        <Bell className="size-4" aria-hidden="true" />
        {unread > 0 && (
          <span className="bg-brand text-2xs absolute -top-0.5 -right-0.5 flex min-w-[1.1rem] items-center justify-center rounded-full px-1 font-mono font-medium text-black">
            {unread > 99 ? '99+' : unread}
          </span>
        )}
      </button>

      {open && (
        <div className="border-line bg-panel absolute right-0 z-50 mt-2 w-[22rem] rounded-lg border shadow-xl">
          <div className="border-line flex items-center justify-between border-b px-4 py-2.5">
            <span className="text-2xs font-medium uppercase tracking-wide">Notifications</span>
            <div className="flex items-center gap-1">
              {notifications.length > 0 && (
                <button
                  type="button"
                  onClick={markAllRead}
                  className="text-mute hover:text-fg flex items-center gap-1 text-2xs"
                >
                  <Check className="size-3" aria-hidden="true" /> Mark read
                </button>
              )}
              <button
                type="button"
                onClick={() => setOpen(false)}
                aria-label="Close notifications"
                className="text-mute hover:text-fg p-1"
              >
                <X className="size-3.5" aria-hidden="true" />
              </button>
            </div>
          </div>

          {notifications.length === 0 ? (
            <div className="text-mute flex flex-col items-center gap-2 px-4 py-8 text-center text-sm">
              <BellOff className="size-5" aria-hidden="true" />
              Nothing has happened yet.
            </div>
          ) : (
            <ul className="max-h-[24rem] overflow-y-auto">
              {notifications.slice(0, 30).map((item) => (
                <Row key={item.id} item={item} />
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
