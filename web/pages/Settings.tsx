import React, { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  Activity,
  KeyRound,
  Server,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  Users,
} from 'lucide-react';

import { useDashboard } from '../hooks/useDashboardData';
import { useAuth } from '../hooks/useAuth';
import {
  ManagedUser,
  changePassword,
  createUser,
  deleteUser,
  fetchHealth,
  fetchUsers,
} from '../services/api';
import { HealthStatus } from '../types';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/card';
import { ScreenHeading } from '../components/ui/section-heading';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';
import { Skeleton } from '../components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs';

type SectionID = 'status' | 'configuration' | 'privacy' | 'users' | 'account';

/** The sections, in the order an operator is likely to want them. */
const SECTIONS: {
  id: SectionID;
  label: string;
  icon: React.ElementType;
  adminOnly?: boolean;
}[] = [
  { id: 'status', label: 'Status', icon: Server },
  { id: 'configuration', label: 'Configuration', icon: SlidersHorizontal },
  { id: 'privacy', label: 'Privacy & integrations', icon: ShieldCheck },
  // Viewers must not see user management at all, rather than seeing it
  // disabled: the server enforces this, and a visible-but-dead control just
  // invites a support question.
  { id: 'users', label: 'Users', icon: Users, adminOnly: true },
  { id: 'account', label: 'Account', icon: KeyRound },
];

const isSectionID = (value: string): value is SectionID =>
  SECTIONS.some((s) => s.id === value);

const Row: React.FC<{ label: string; value: React.ReactNode; hint?: string }> = ({
  label,
  value,
  hint,
}) => (
  // Stacks on phones. Config keys like `collector.poll_interval_seconds` are
  // single unbreakable tokens, so a fixed 170px label column left them nowhere
  // to go and they ran straight off the side of the screen.
  <div className="grid grid-cols-1 gap-1 border-b py-2.5 last:border-0 sm:grid-cols-[170px_1fr] sm:gap-4">
    <dt className="text-mute pt-0.5 text-2xs tracking-[0.15em] uppercase">{label}</dt>
    <dd className="min-w-0 text-sm">
      <span className="tabular block break-all">{value}</span>
      {hint && <p className="text-mute mt-0.5 text-xs">{hint}</p>}
    </dd>
  </div>
);

const Settings: React.FC = () => {
  const { current, feed, hosts } = useDashboard();
  const { user } = useAuth();
  const isAdmin = user?.role === 'admin';

  const [health, setHealth] = useState<HealthStatus | null>(null);
  const [loaded, setLoaded] = useState(false);

  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [usersError, setUsersError] = useState<string | null>(null);
  const [newEmail, setNewEmail] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState<'admin' | 'viewer'>('viewer');
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);

  const [currentPw, setCurrentPw] = useState('');
  const [nextPw, setNextPw] = useState('');
  const [pwError, setPwError] = useState<string | null>(null);
  const [pwDone, setPwDone] = useState(false);

  // Guarding on the role keeps a viewer from firing a request that can only
  // ever come back 403.
  const adminCount = users.filter((u) => u.role === 'admin').length;

  const loadUsers = useCallback(async () => {
    if (!isAdmin) return;
    try {
      setUsers(await fetchUsers());
      setUsersError(null);
    } catch (err) {
      setUsersError(err instanceof Error ? err.message : 'Failed to load users');
    }
  }, [isAdmin]);

  useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  const addUser = async () => {
    try {
      await createUser(newEmail, newPassword, newRole);
      setNewEmail('');
      setNewPassword('');
      setNewRole('viewer');
      setUsersError(null);
      await loadUsers();
    } catch (err) {
      setUsersError(err instanceof Error ? err.message : 'Failed to create user');
    }
  };

  const removeUser = async (id: string) => {
    try {
      await deleteUser(id);
      setPendingDelete(null);
      setUsersError(null);
      await loadUsers();
    } catch (err) {
      setUsersError(err instanceof Error ? err.message : 'Failed to delete user');
    }
  };

  const submitPassword = async () => {
    setPwError(null);
    setPwDone(false);
    try {
      await changePassword(currentPw, nextPw);
      setCurrentPw('');
      setNextPw('');
      setPwDone(true);
    } catch (err) {
      setPwError(err instanceof Error ? err.message : 'Password change failed');
    }
  };

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      const h = await fetchHealth();
      if (!cancelled) {
        setHealth(h);
        setLoaded(true);
      }
    };
    load();
    const id = setInterval(load, 10000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  // The active section lives in the URL hash rather than in component state
  // alone, so a section can be linked to and survives a reload. The account
  // menu already links to /settings#account, and that keeps working unchanged.
  const location = useLocation();
  const navigate = useNavigate();

  const fromHash = location.hash.replace(/^#/, '');
  const section: SectionID = isSectionID(fromHash) ? fromHash : 'status';

  const selectSection = useCallback(
    (next: string) => {
      // replace, not push: flipping between tabs should not fill the back
      // button with settings sections.
      navigate({ hash: next }, { replace: true });
    },
    [navigate],
  );

  const statusVariant =
    health?.status === 'healthy' ? 'ok' : health?.status === 'degraded' ? 'warn' : 'crit';

  return (
    <>
      <ScreenHeading
        eyebrow="System administration"
        title="Settings"
        description="Daemon status, configuration reference, the privacy boundary, and who is allowed to operate this console."
      />

      {/* Tabs rather than one long scroll. Six panels stacked in a
          two-column grid put the change-password form beside the API endpoint
          reference, which is not a relationship that exists. Each section is
          also addressable: the account menu links to /settings#account, and
          that keeps working because the tab reads the hash. */}
      <Tabs value={section} onValueChange={selectSection}>
        <TabsList aria-label="Settings sections">
          {SECTIONS.filter((s) => !s.adminOnly || isAdmin).map((s) => (
            <TabsTrigger key={s.id} value={s.id}>
              <s.icon className="size-3.5" aria-hidden="true" />
              {s.label}
            </TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value="status" className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <Card>
            <CardHeader>
              <Server className="text-muted-foreground size-4" />
              <CardTitle>Daemon</CardTitle>
              {health && (
                <Badge variant={statusVariant} className="ml-auto">
                  {health.status}
                </Badge>
              )}
            </CardHeader>
            <CardContent>
              {!loaded ? (
                <div className="space-y-2">
                  <Skeleton className="h-5 w-full" />
                  <Skeleton className="h-5 w-4/5" />
                  <Skeleton className="h-5 w-3/5" />
                </div>
              ) : !health ? (
                <p className="text-muted-foreground py-6 text-center text-sm">
                  Cannot reach the daemon — /health did not respond.
                </p>
              ) : (
                <dl>
                  <Row
                    label="Version"
                    value={health.version || 'unknown'}
                    hint={health.commit ? `commit ${health.commit}` : undefined}
                  />
                  <Row label="Database" value={health.database} />
                  <Row
                    label="Collector"
                    value={health.collector ?? 'unknown'}
                    hint={
                      health.lastSampleAgeSeconds !== undefined
                        ? `last sample ${health.lastSampleAgeSeconds}s ago`
                        : undefined
                    }
                  />
                  <Row label="Dashboard clients" value={health.websocketClients ?? '—'} />
                  <Row label="Host" value={current.hostname || 'unknown'} />
                  {/* No `|| 1` fallback: the hosts table is populated in
                      every mode now, so a zero here means something is
                      genuinely wrong rather than a known gap being hidden. */}
                  <Row label="Fleet" value={`${hosts.length} host${hosts.length === 1 ? '' : 's'}`} />
                  <Row label="Feed" value={`${feed.label} — ${feed.detail}`} />
                </dl>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="configuration" className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <Card>
            <CardHeader>
              <SlidersHorizontal className="text-muted-foreground size-4" />
              <CardTitle>Configuration</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground mb-3 text-sm">
                Settings are not yet editable from the browser. Configure the daemon with{' '}
                <code className="bg-muted rounded px-1 py-0.5 text-xs">config.yaml</code> or{' '}
                <code className="bg-muted rounded px-1 py-0.5 text-xs">SYS_SENTIENT_*</code> environment
                variables, then restart it. See{' '}
                <code className="bg-muted rounded px-1 py-0.5 text-xs">config.yaml.example</code>.
              </p>
              <dl className="font-mono">
                <Row label="Mode" value="mode" hint="all-in-one, server or agent." />
                <Row label="Poll interval" value="collector.poll_interval_seconds" hint="Default 2s." />
                <Row label="Top processes" value="collector.top_processes" hint="Default 10." />
                <Row label="Log level" value="logging.level" hint="debug adds one line per sample." />
                <Row label="Retention" value="database.metrics_retention_hours" hint="Default 24h." />
                <Row label="Alert webhook" value="alerting.webhook_url" hint="Raw alert JSON per transition." />
                <Row label="AI spend cap" value="gemini.max_daily_cost" hint="Hard USD cap per UTC day." />
              </dl>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="privacy" className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <Card>
            <CardHeader>
              <ShieldCheck className="text-muted-foreground size-4" />
              <CardTitle>Privacy</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground text-sm leading-relaxed">
                With no Gemini API key configured, nothing leaves this machine — there is no telemetry
                or analytics of any kind. When AI analysis is enabled, the current metric sample and
                recent logs are sent to Google. IPv4 and IPv6 addresses, e-mail addresses and
                home-directory usernames are redacted from logs, process names and the top-process
                summary first.
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <Activity className="text-muted-foreground size-4" />
              <CardTitle>Integrations</CardTitle>
            </CardHeader>
            <CardContent>
              <dl>
                <Row
                  label="Prometheus"
                  value={
                    <a className="text-primary hover:underline" href="/metrics">
                      /metrics
                    </a>
                  }
                  hint="Includes the daemon's own goroutine, heap and GC stats."
                />
                <Row
                  label="Health"
                  value={
                    <a className="text-primary hover:underline" href="/health">
                      /health
                    </a>
                  }
                  hint="Version, database and collector liveness. 503 when degraded."
                />
                <Row
                  label="Ingest"
                  value={<code className="text-xs">POST /api/ingest</code>}
                  hint="Where agents push samples. Uses the separate agent key."
                />
              </dl>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="users">
          {isAdmin && (
            <Card id="users" className="lg:col-span-2">
              <CardHeader>
                <Users className="text-muted-foreground size-4" />
                <CardTitle>Users</CardTitle>
                <Badge variant="secondary" className="ml-auto tabular">
                  {users.length}
                </Badge>
              </CardHeader>
              <CardContent>
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[560px] text-sm">
                    <thead>
                      <tr className="text-muted-foreground border-b text-2xs tracking-wide uppercase">
                        <th scope="col" className="px-1 py-2 text-left font-medium">Email</th>
                        <th scope="col" className="px-1 py-2 text-left font-medium">Role</th>
                        <th scope="col" className="px-1 py-2 text-left font-medium">Last sign-in</th>
                        <th scope="col" className="px-1 py-2 text-right font-medium">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {users.map((u) => {
                        const isSelf = u.id === user?.id;
                        const isLastAdmin = u.role === 'admin' && adminCount <= 1;
                        const blocked = isSelf
                          ? 'You cannot delete your own account'
                          : isLastAdmin
                            ? 'The last admin cannot be deleted'
                            : '';
                        return (
                          <tr key={u.id} className="border-b last:border-0">
                            <td className="px-1 py-2.5">{u.email}</td>
                            <td className="px-1 py-2.5">
                              <Badge variant={u.role === 'admin' ? 'ok' : 'outline'} className="px-2 py-0">
                                {u.role}
                              </Badge>
                            </td>
                            <td className="text-mute tabular px-1 py-2.5 text-xs">
                              {u.lastLoginAt ? new Date(u.lastLoginAt).toLocaleString() : 'never'}
                            </td>
                            <td className="px-1 py-2.5 text-right">
                              {pendingDelete === u.id ? (
                                <span className="inline-flex gap-2">
                                  <Button
                                    size="sm"
                                    variant="destructive"
                                    aria-label={`Confirm delete ${u.email}`}
                                    onClick={() => void removeUser(u.id)}
                                  >
                                    Confirm delete
                                  </Button>
                                  <Button size="sm" variant="ghost" onClick={() => setPendingDelete(null)}>
                                    Cancel
                                  </Button>
                                </span>
                              ) : (
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  aria-label={`Delete ${u.email}`}
                                  disabled={Boolean(blocked)}
                                  title={blocked || undefined}
                                  onClick={() => setPendingDelete(u.id)}
                                >
                                  <Trash2 className="size-4" aria-hidden="true" />
                                </Button>
                              )}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>

                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    void addUser();
                  }}
                  className="mt-5 grid gap-3 border-t pt-4 sm:grid-cols-[1fr_1fr_auto_auto]"
                >
                  <div className="grid gap-1.5">
                    <Label htmlFor="new-user-email">Email</Label>
                    <Input
                      id="new-user-email"
                      type="email"
                      autoComplete="off"
                      required
                      value={newEmail}
                      onChange={(e) => setNewEmail(e.target.value)}
                    />
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="new-user-password">Password</Label>
                    <Input
                      id="new-user-password"
                      type="password"
                      autoComplete="new-password"
                      required
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      aria-describedby="new-user-password-hint"
                    />
                    <p id="new-user-password-hint" className="text-mute text-xs">
                      At least 12 characters.
                    </p>
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="new-user-role">Role</Label>
                    <select
                      id="new-user-role"
                      value={newRole}
                      onChange={(e) => setNewRole(e.target.value === 'admin' ? 'admin' : 'viewer')}
                      className="border-input bg-transparent h-9 rounded-md border px-2 text-sm"
                    >
                      <option value="viewer">viewer</option>
                      <option value="admin">admin</option>
                    </select>
                  </div>
                  <div className="flex items-end">
                    <Button type="submit">Add user</Button>
                  </div>
                  {usersError && (
                    <p role="alert" className="text-crit text-sm sm:col-span-4">
                      {usersError}
                    </p>
                  )}
                </form>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="account">
          <Card id="account" className="lg:col-span-2">
            <CardHeader>
              <KeyRound className="text-muted-foreground size-4" />
              <CardTitle>Account</CardTitle>
              {user && (
                <Badge variant="outline" className="ml-auto uppercase">
                  {user.role}
                </Badge>
              )}
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground mb-4 text-sm">
                Signed in as <span className="text-foreground">{user?.email ?? 'unknown'}</span>.
              </p>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void submitPassword();
                }}
                className="grid max-w-md gap-3"
              >
                <div className="grid gap-1.5">
                  <Label htmlFor="current-password">Current password</Label>
                  <Input
                    id="current-password"
                    type="password"
                    autoComplete="current-password"
                    required
                    value={currentPw}
                    onChange={(e) => setCurrentPw(e.target.value)}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="next-password">New password</Label>
                  <Input
                    id="next-password"
                    type="password"
                    autoComplete="new-password"
                    required
                    value={nextPw}
                    onChange={(e) => setNextPw(e.target.value)}
                    aria-describedby="next-password-hint"
                  />
                  <p id="next-password-hint" className="text-mute text-xs">
                    At least 12 characters.
                  </p>
                </div>
                <div>
                  <Button type="submit">Change password</Button>
                </div>
                {pwError && (
                  <p role="alert" className="text-crit text-sm">
                    {pwError}
                  </p>
                )}
                {pwDone && (
                  <p role="status" className="text-ok text-sm">
                    Password updated. Other devices have been signed out.
                  </p>
                )}
              </form>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  );
};

export default Settings;
