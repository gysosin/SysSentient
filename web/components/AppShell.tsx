import React from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { AnimatePresence, motion } from 'motion/react';
import {
  Server,
  AlertTriangle,
  Cpu,
  FileText,
  Keyboard,
  LayoutDashboard,
  Moon,
  Pause,
  Play,
  Settings as SettingsIcon,
  Sparkles,
  Sun,
  X,
} from 'lucide-react';

import { FeedStatus, FleetHost } from '../types';
import { AuthUser } from '../services/api';
import { cn } from '../lib/utils';
import { NotificationCenter } from './NotificationCenter';
import { RangePicker } from './RangePicker';
import { useFeed } from '../hooks/useDashboardData';
import { UserMenu } from './UserMenu';
import { Button } from './ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './ui/select';

const NAV = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/processes', label: 'Processes', icon: Cpu },
  { to: '/hosts', label: 'Hosts', icon: Server },
  { to: '/logs', label: 'Logs', icon: FileText },
  { to: '/insights', label: 'Insights', icon: Sparkles },
  { to: '/alerts', label: 'Alerts', icon: AlertTriangle },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
];

/**
 * The feed indicator's colour vocabulary.
 *
 * `live` is deliberately brand coral rather than green. Green here would read
 * as "healthy machine", and the feed badge says nothing about the machine — it
 * says whether the numbers on screen can be trusted at all. Those are different
 * claims, and conflating them is how a dashboard shows confident figures about
 * a host that stopped reporting ten minutes ago.
 */
const FEED_TONE: Record<FeedStatus['level'], { dot: string; chip: string }> = {
  live: { dot: 'bg-brand live-pulse', chip: 'border-brand/40 bg-brand-soft text-brand' },
  polling: { dot: 'bg-warn', chip: 'border-warn/40 bg-warn-soft text-warn' },
  stale: { dot: 'bg-crit', chip: 'border-crit/40 bg-crit-soft text-crit' },
  // Also crit, not grey. "Nothing has ever arrived" is a failure, and a muted
  // chip makes it look like an idle machine — the one reading the brief
  // explicitly rules out. Stale and down share a severity; the label is what
  // separates "the numbers stopped" from "there were never any numbers".
  down: { dot: 'bg-crit', chip: 'border-crit/40 bg-crit-soft text-crit' },
};

/** Wall-clock of the most recent sample, in UTC. */
function lastSyncLabel(ageMs: number): string {
  if (!Number.isFinite(ageMs) || ageMs < 0) return '--:--:--';
  return new Date(Date.now() - ageMs).toISOString().slice(11, 19);
}

/**
 * The live/stale chip.
 *
 * Its own component because it reads the feed, which republishes every second.
 * AppShell wraps the routed page, so if AppShell subscribed to the feed the
 * whole page -- charts included -- would re-render once a second to update this
 * one string.
 */
const FeedChip: React.FC = () => {
  const feed = useFeed();
  const tone = FEED_TONE[feed.level];
  return (
    <div
      className={cn(
        'flex items-center gap-2 rounded-md border px-2.5 py-2 text-2xs font-semibold tracking-wider',
        tone.chip,
      )}
      role="status"
      aria-live="polite"
    >
      <span className={cn('size-2 shrink-0 rounded-full', tone.dot)} aria-hidden="true" />
      <span>{feed.label}</span>
      <span className="text-mute hidden font-mono font-normal sm:inline">{feed.detail}</span>
    </div>
  );
};

/** The last-sync clock, isolated for the same reason as FeedChip. */
const LastSync: React.FC = () => {
  const feed = useFeed();
  return <span className="text-foreground">{lastSyncLabel(feed.ageMs)} UTC</span>;
};

/** The stale/no-data banner, isolated for the same reason as FeedChip. */
const DegradedBanner: React.FC = () => {
  const feed = useFeed();
  const [dismissed, setDismissed] = React.useState(false);
  const degraded = feed.level === 'stale' || feed.level === 'down';

  // A dismissed banner must come back when the feed recovers and breaks again,
  // otherwise one click permanently silences the console's most important
  // warning.
  React.useEffect(() => {
    if (!degraded) setDismissed(false);
  }, [degraded]);

  return (
    <AnimatePresence>
      {degraded && !dismissed && (
        <motion.div
          initial={{ height: 0, opacity: 0 }}
          animate={{ height: 'auto', opacity: 1 }}
          exit={{ height: 0, opacity: 0 }}
          className={cn(
            'overflow-hidden border-b',
            feed.level === 'stale'
              ? 'border-warn/40 bg-warn-soft text-warn'
              : 'border-crit/40 bg-crit-soft text-crit',
          )}
          role="alert"
        >
          <div className="flex w-full items-center gap-3 px-4 py-2.5 text-2xs sm:px-6">
            <AlertTriangle className="size-4 shrink-0" aria-hidden="true" />
            <span className="min-w-0">
              <strong className="tracking-wider uppercase">
                {feed.level === 'stale' ? 'Stale data' : 'No data'}
              </strong>{' '}
              {feed.level === 'stale'
                ? `Feed stale — ${feed.detail}. The values below are not current.`
                : 'No data received from the daemon. Check that sys-daemon is running and reachable.'}
            </span>
            <button
              type="button"
              onClick={() => setDismissed(true)}
              className="ml-auto shrink-0 rounded p-1 hover:bg-current/10"
              aria-label="Dismiss feed warning"
            >
              <X className="size-4" />
            </button>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  );
};

interface Props {
  hostname: string;
  uptimeLabel: string;
  hosts: FleetHost[];
  selectedHost: string;
  onSelectHost: (hostID: string) => void;
  firingAlerts: number;
  user: AuthUser | null;
  onSignOut: () => void;
  frozen: boolean;
  onToggleFreeze: () => void;
  theme: 'dark' | 'light';
  onToggleTheme: () => void;
  onShowShortcuts: () => void;
}

function NavItems({
  variant,
  firingAlerts,
}: {
  variant: 'bar' | 'chips' | 'tabs';
  firingAlerts: number;
}) {
  const location = useLocation();
  return (
    <>
      {NAV.map((item) => {
        const Icon = item.icon;
        const active = item.end
          ? location.pathname === item.to
          : location.pathname.startsWith(item.to);

        return (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            aria-current={active ? 'page' : undefined}
            className={cn(
              'font-medium transition-colors',
              variant === 'tabs'
                ? // A phone target, not a link: full height, centred, and wide
                  // enough for a thumb rather than a cursor.
                  'relative flex min-h-14 flex-col items-center justify-center gap-1 text-2xs tracking-wide'
                : 'flex shrink-0 items-center gap-2 text-2xs',
              variant === 'bar' && 'border-b-2 pb-1.5',
              variant === 'chips' && 'rounded-md border px-3 py-2',
              active
                ? variant === 'bar'
                  ? 'border-brand text-foreground'
                  : variant === 'tabs'
                    ? 'text-brand'
                    : 'border-line bg-panel-strong text-foreground'
                : cn(
                    variant === 'tabs' ? 'text-mute' : 'text-mute hover:text-foreground',
                    variant !== 'tabs' && 'border-transparent',
                  ),
            )}
          >
            {/* The active tab is marked by a rule at the top of the cell as
                well as by colour, so the current section is still identifiable
                without colour vision. */}
            {variant === 'tabs' && active && (
              <span
                className="bg-brand absolute inset-x-3 top-0 h-0.5 rounded-full"
                aria-hidden="true"
              />
            )}
            <span className={cn('relative', variant === 'tabs' && 'block')}>
              <Icon className={variant === 'tabs' ? 'size-[18px]' : 'size-3.5'} aria-hidden="true" />
              {/* On the tab bar the count rides the icon: there is no room for
                  a chip beside a 9px label. */}
              {variant === 'tabs' && item.to === '/alerts' && firingAlerts > 0 && (
                <span
                  className="bg-crit text-background absolute -top-1.5 -right-2 grid min-w-4 place-items-center rounded-full px-1 font-mono text-2xs font-bold"
                  aria-label={`${firingAlerts} firing`}
                >
                  {firingAlerts}
                </span>
              )}
            </span>
            <span className={cn(variant === 'tabs' && 'leading-none')}>{item.label}</span>
            {/* Often the only signal that anything needs attention, so it has
                to be impossible to miss without being obnoxious on a display
                that is left open all day. */}
            {variant !== 'tabs' && item.to === '/alerts' && firingAlerts > 0 && (
              <span
                className="bg-crit-soft text-crit tabular rounded-full px-1.5 py-0.5 font-mono text-2xs font-bold"
                aria-label={`${firingAlerts} firing`}
              >
                {firingAlerts}
              </span>
            )}
          </NavLink>
        );
      })}
    </>
  );
}

const AppShell: React.FC<Props> = ({
  hostname,
  uptimeLabel,
  hosts,
  selectedHost,
  onSelectHost,
  firingAlerts,
  user,
  onSignOut,
  frozen,
  onToggleFreeze,
  theme,
  onToggleTheme,
  onShowShortcuts,
}) => {
  const location = useLocation();

  return (
    <div className="gridbg text-foreground flex min-h-screen flex-col">
      <a href="#main" className="skip-link text-sm">
        Skip to content
      </a>

      <header className="bg-background/95 border-line sticky top-0 z-50 border-b backdrop-blur-sm">
        <div className="flex min-h-16 w-full flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2.5 sm:px-6 2xl:px-10">
          {/* The mark carries the product's identity in the display face, with
              the deployment model stated underneath — this is sold as
              self-hosted, and the header is where that is claimed. */}
          <div className="flex min-w-0 items-center gap-2.5 sm:gap-3">
            <span className="border-brand/40 bg-brand-soft text-brand font-display grid size-8 shrink-0 place-items-center rounded-md border text-base font-bold tracking-tight sm:size-9 sm:text-lg">
              SS
            </span>
            <span className="min-w-0 leading-none">
              {/* Tracking is the first thing to give up on a narrow screen: at
                  0.18em the wordmark alone pushed the feed chip onto a second
                  row, which cost more than the letterspacing was worth. */}
              <span className="font-display block truncate text-xs font-semibold tracking-[0.1em] uppercase sm:text-[15px] sm:tracking-[0.18em]">
                SysSentient
              </span>
              <span className="text-mute mt-1 hidden truncate text-2xs tracking-[0.28em] uppercase sm:block">
                Self-hosted console
              </span>
            </span>
          </div>

          <div className="ml-auto flex items-center gap-2.5 sm:gap-3">
            <FeedChip />

            {/* The window every chart draws. Beside the feed chip because the
                two together answer "what am I looking at, and is it current". */}
            <RangePicker />

            {/* Freeze pins the view without pausing collection, so a spike can
                be read instead of scrolling away. Deliberately next to the feed
                chip: the two together answer "can I trust this number". */}
            <Button
              variant={frozen ? 'accent' : 'outline'}
              size="sm"
              onClick={onToggleFreeze}
              aria-pressed={frozen}
              title={frozen ? 'Resume the live stream (Space)' : 'Freeze the view (Space)'}
            >
              {frozen ? <Play aria-hidden="true" /> : <Pause aria-hidden="true" />}
              <span className="hidden md:inline">{frozen ? 'Frozen' : 'Freeze'}</span>
            </Button>

            <Button
              variant="ghost"
              size="icon"
              onClick={onToggleTheme}
              title={`Switch to the ${theme === 'dark' ? 'light' : 'dark'} theme (T)`}
              aria-label={`Switch to the ${theme === 'dark' ? 'light' : 'dark'} theme`}
              className="hidden sm:inline-flex"
            >
              {theme === 'dark' ? <Sun aria-hidden="true" /> : <Moon aria-hidden="true" />}
            </Button>

            <Button
              variant="ghost"
              size="icon"
              onClick={onShowShortcuts}
              title="Keyboard shortcuts (?)"
              aria-label="Keyboard shortcuts"
              className="hidden lg:inline-flex"
            >
              <Keyboard aria-hidden="true" />
            </Button>

            {/* Only shown once a fleet exists; a single-node install keeps the
                plain hostname it always had. */}
            {/* Shown even with one host: the switcher used to appear only once a
                second machine existed, so the concept -- and the path to adding
                one -- were invisible on every single-node install. */}
            {hosts.length >= 1 ? (
              <Select
                value={selectedHost || '__all__'}
                onValueChange={(v) => onSelectHost(v === '__all__' ? '' : v)}
              >
                <SelectTrigger size="sm" aria-label="Select host" className="min-w-[11rem]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">All hosts ({hosts.length})</SelectItem>
                  {hosts.map((h) => (
                    <SelectItem key={h.hostId} value={h.hostId}>
                      {h.hostname} · {h.hostId.slice(0, 8)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <span className="text-foreground hidden font-mono text-2xs lg:inline">
                {hostname}
              </span>
            )}

            {/* A bell, where one is expected. Alerts previously surfaced only
                on their own page and as a count on a nav link, so anything
                happening while you looked elsewhere went unnoticed. */}
            <NotificationCenter />
            {user && <UserMenu user={user} onSignOut={onSignOut} />}
          </div>
        </div>

        {/* Tablet only. Six chips fit across 768px without scrolling; below
            that they would overflow and hide the last two sections, so phones
            get the bottom tab bar instead. */}
        <nav
          aria-label="Sections"
          className="hidden w-full gap-1 px-4 pb-2 sm:px-6 md:flex lg:hidden"
        >
          <NavItems variant="chips" firingAlerts={firingAlerts} />
        </nav>
      </header>

      <div
        className="border-line hidden w-full items-center gap-5 overflow-x-auto border-b px-6 py-2.5 lg:flex 2xl:px-10"
        aria-label="Sections"
      >
        <NavItems variant="bar" firingAlerts={firingAlerts} />
        {/* Uptime and last-sync are the two facts that qualify everything else
            on screen, so they sit in the chrome rather than inside any one
            panel. Mono keeps them from reflowing as the clock ticks. */}
        <span className="text-melt tabular ml-auto flex shrink-0 items-center gap-2 font-mono text-2xs tracking-wider uppercase">
          Uptime <span className="text-foreground">{uptimeLabel}</span>
          <span className="text-line">/</span>
          Last sync <LastSync />
        </span>
      </div>

      <AnimatePresence>
        {frozen && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            className="border-accent/40 bg-accent/10 text-accent overflow-hidden border-b"
            role="status"
          >
            <div className="flex w-full items-center gap-3 px-4 py-2.5 text-2xs sm:px-6">
              <Pause className="size-4 shrink-0" aria-hidden="true" />
              <span className="min-w-0">
                <strong className="tracking-wider uppercase">Frozen</strong> You are holding the
                view still. The daemon is still collecting; these values are from the moment you
                froze.
              </span>
              <button
                type="button"
                onClick={onToggleFreeze}
                className="ml-auto shrink-0 font-semibold underline underline-offset-2"
              >
                Resume
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      <DegradedBanner />

      {/* pb-24 on phones keeps the last panel clear of the fixed tab bar. */}
      <main
        id="main"
        className="w-full flex-1 px-4 py-6 pb-24 sm:px-6 md:pb-6 lg:py-7 2xl:px-10"
      >
        <AnimatePresence mode="wait">
          <motion.div
            key={location.pathname}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={{ duration: 0.2, ease: [0.22, 1, 0.36, 1] }}
          >
            <Outlet />
          </motion.div>
        </AnimatePresence>
      </main>

      <footer className="border-line text-mute flex w-full flex-wrap items-center justify-between gap-3 border-t px-6 py-5 pb-24 text-2xs md:pb-5">
        <div className="flex flex-wrap items-center gap-4">
          <span className="flex items-center gap-1.5">
            <span
              className={cn(
                'size-1.5 rounded-full',
                hosts.length > 0 ? 'bg-ok' : 'bg-melt',
              )}
              aria-hidden="true"
            />
            {hosts.length > 0
              ? `${hosts.length} agent${hosts.length === 1 ? '' : 's'} reporting`
              : 'no agents enrolled'}
          </span>
          <span className="font-mono">self-hosted</span>
        </div>
      </footer>

      {/* Phone navigation. A thumb-reachable bar beats a scrolling strip at the
          top of the page: all six sections are visible at once, nothing hides
          off the right edge, and the alert count sits in the one place someone
          checking their phone will actually look. */}
      <nav
        aria-label="Sections"
        className="border-line bg-background/95 fixed inset-x-0 bottom-0 z-50 grid grid-cols-6 border-t backdrop-blur-sm md:hidden"
        style={{ paddingBottom: 'env(safe-area-inset-bottom)' }}
      >
        <NavItems variant="tabs" firingAlerts={firingAlerts} />
      </nav>
    </div>
  );
};

export default AppShell;
