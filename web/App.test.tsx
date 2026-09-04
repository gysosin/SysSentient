import assert from 'node:assert/strict';
import { act } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, test, vi } from 'vitest';

// NOTE: this file lives at the web/ root on purpose. Under the previous
// `node --test .tmp-tests/**/*.test.js` harness, `**` expanded as a single `*`
// under sh, so a test here was silently skipped with a green exit code.

// Spread the real module first: a wholesale mock breaks every time an export
// is added, and the failure surfaces as an unhandled rejection rather than a
// failed assertion. Only the calls this suite needs to control are replaced.
vi.mock('./services/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./services/api')>()),
  fetchMetricsHistory: vi.fn(async () => ({ metrics: [], processes: [] })),
  fetchRecentLogs: vi.fn(async () => []),
  fetchInsightHistory: vi.fn(async () => []),
  fetchAlertHistory: vi.fn(async () => []),
  triggerAnalysis: vi.fn(async () => {
    throw new Error('not used');
  }),
  // The console is behind a login now, so every test starts from a resolved
  // admin session; without these the app renders the login page instead.
  fetchMe: vi.fn(async () => ({ id: 'u1', email: 'admin@example.com', role: 'admin' })),
  fetchSetupStatus: vi.fn(async () => false),
  onUnauthorized: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(async () => undefined),
  completeSetup: vi.fn(),
  fetchActiveAlerts: vi.fn(async () => []),
  fetchHosts: vi.fn(async () => []),
  fetchHealth: vi.fn(async () => null),
}));

import App from './App';

type Listener = ((event: unknown) => void) | null;

class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readyState = FakeWebSocket.CONNECTING;
  onopen: Listener = null;
  onmessage: Listener = null;
  onclose: Listener = null;
  onerror: Listener = null;

  constructor(public url: string) {
    FakeWebSocket.instances.push(this);
  }

  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.({ code: 1000 });
  }

  accept() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.({});
  }

  send(payload: unknown) {
    this.onmessage?.({ data: JSON.stringify({ type: 'metrics', payload }) });
  }
}

const frameAt = (iso: string) => ({
  timestamp: iso,
  cpu_usage: 42,
  cpu_per_core: [10, 20, 30, 40],
  memory_used: 8 * 1024 * 1024 * 1024,
  memory_total: 16 * 1024 * 1024 * 1024,
  swap_used: 1024 * 1024 * 1024,
  swap_total: 8 * 1024 * 1024 * 1024,
  load_avg_1: 1.5,
  load_avg_5: 2.5,
  load_avg_15: 3.5,
  temperature: 65,
  uptime_seconds: 90061,
  processes: [
    { pid: 4242, name: 'chrome', user: 'xyfo', cpu: 12.5, memory: 512, state: 'Running' },
  ],
});

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal('WebSocket', FakeWebSocket);
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const goTo = (path: string) => window.history.pushState({}, '', path);

/** The console opens its socket only once the session resolves, so a test
 *  cannot assume the connection exists the moment render() returns. */
const awaitSocket = async (): Promise<FakeWebSocket> => {
  await waitFor(() => {
    assert.ok(FakeWebSocket.instances[0], 'expected the app to open a WebSocket');
  });
  return FakeWebSocket.instances[0];
};

describe('App reports the truth about its data feed', () => {
  test('renders live process rows and derived percentages from the socket', async () => {
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'));
    goTo('/');
    render(<App />);
    const socket = await awaitSocket();

    await act(async () => {
      socket.accept();
      socket.send(frameAt('2026-01-01T00:00:00Z'));
    });

    // memory_total is now used as a denominator instead of being discarded.
    assert.ok(screen.getAllByText('50%').length > 0, 'RAM should render as a percentage of total');
    // load_avg_5 / load_avg_15 were collected and never shown.
    assert.ok(screen.getByText(/5m 2\.50/), 'load 5m should render');
    assert.ok(screen.getByText(/15m 3\.50/), 'load 15m should render');
    // temperature was fed to the AI but never to the dashboard.
    assert.ok(screen.getByText('65°C'), 'temperature should render');
    // Real host uptime, not a client-side counter that resets on refresh.
    assert.ok(screen.getAllByText(/1d/).length > 0, 'host uptime should render');

    assert.ok(screen.getByText('LIVE'), 'feed should read LIVE while fresh');
  });

  test('flags the feed as stale when frames stop arriving', async () => {
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'));
    goTo('/');
    render(<App />);
    const socket = await awaitSocket();

    await act(async () => {
      socket.accept();
      socket.send(frameAt('2026-01-01T00:00:00Z'));
    });
    assert.ok(screen.getByText('LIVE'), 'should start LIVE');

    // The socket stays "open" but goes silent — the half-open case that used to
    // freeze the dashboard on stale numbers while still showing LIVE.
    await act(async () => {
      vi.setSystemTime(new Date('2026-01-01T00:01:00Z'));
      vi.advanceTimersByTime(2000);
    });

    assert.ok(screen.getByText('STALE'), 'feed must be reported as stale');
    assert.ok(
      screen.getByRole('alert').textContent?.includes('not current'),
      'a banner must warn the values are not current',
    );
  });

  test('says so when no data has ever arrived', async () => {
    goTo('/');
    render(<App />);

    await act(async () => {
      await Promise.resolve();
    });

    assert.ok(screen.getByText('NO DATA'));
    assert.ok(
      screen.getByRole('alert').textContent?.includes('No data received'),
      'must distinguish "never received" from "idle system"',
    );
  });
});

describe('freezing the stream', () => {
  // Freeze must pin the view without pretending the daemon stopped. Pausing
  // collection instead of snapshotting would make the feed read STALE, and
  // "the operator asked me to hold still" looking identical to "the machine
  // stopped answering" would undermine the one thing this console sells.
  test('holds the displayed values while the feed keeps running', async () => {
    goTo('/processes');
    render(<App />);
    const socket = await awaitSocket();

    await act(async () => {
      // Without accept() the hook falls back to polling and the socket frames
      // never reach the table.
      socket.accept();
      socket.send({ ...frameAt(new Date().toISOString()), cpu_usage: 42 });
    });
    await screen.findByText('chrome');

    const freeze = await screen.findByRole('button', { name: /freeze/i });
    await act(async () => {
      fireEvent.click(freeze);
    });

    // A later frame with a different process must not reach the table.
    await act(async () => {
      socket.send({
        ...frameAt(new Date().toISOString()),
        cpu_usage: 91,
        processes: [
          { pid: 777, name: 'postgres', user: 'pgadmin', cpu: 88, memory: 2048, state: 'Running' },
        ],
      });
    });

    assert.ok(screen.getByText('chrome'), 'the frozen view must keep the snapshot');
    assert.equal(
      screen.queryByText('postgres'),
      null,
      'a frame that arrived after freezing must not be displayed',
    );

    // ...and the feed badge must still describe the daemon, not the snapshot.
    assert.ok(screen.getByText('LIVE'), 'freezing must not make the feed look stale');

    // Resuming catches up.
    const resume = await screen.findByRole('button', { name: /frozen/i });
    await act(async () => {
      fireEvent.click(resume);
    });
    await waitFor(() => {
      assert.ok(screen.getByText('postgres'), 'resuming must show the latest frame');
    });
  });
});

describe('navigation reaches every section', () => {
  // The phone nav was a horizontally scrolling strip, which pushed Alerts and
  // Settings off the right edge of a 375px screen — including the firing-alert
  // count, which is the single reason someone opens this on a phone. Every
  // section must be present as a link at every breakpoint; CSS decides which
  // of the three navigations is visible, never which sections exist.
  const SECTIONS: [string, string][] = [
    ['Overview', '/'],
    ['Processes', '/processes'],
    ['Logs', '/logs'],
    ['Insights', '/insights'],
    ['Alerts', '/alerts'],
    ['Settings', '/settings'],
  ];

  test('every section is a reachable link from the shell', async () => {
    goTo('/');
    render(<App />);

    await act(async () => {
      await Promise.resolve();
    });

    for (const [label, href] of SECTIONS) {
      const links = screen.getAllByRole('link', { name: new RegExp(`^${label}`) });
      assert.ok(links.length > 0, `no navigation link for ${label}`);
      assert.ok(
        links.some((link) => link.getAttribute('href') === href),
        `${label} does not point at ${href}`,
      );
    }
  });
});

describe('Settings sections', () => {
  // The account menu links to /settings#account. When Settings became tabbed
  // that link had to keep working, or the only route to the change-password
  // form would have silently become a dead end.
  test('the account deep link opens the Account section', async () => {
    goTo('/settings#account');
    render(<App />);

    await act(async () => {
      await Promise.resolve();
    });

    await waitFor(() => {
      const active = document.querySelector('[data-slot="tabs-trigger"][data-state="active"]');
      assert.ok(active, 'no active settings tab');
      assert.match(active.textContent ?? '', /Account/);
    });
  });

  test('an unknown hash falls back to the first section rather than a blank page', async () => {
    goTo('/settings#nonsense');
    render(<App />);

    await act(async () => {
      await Promise.resolve();
    });

    await waitFor(() => {
      const active = document.querySelector('[data-slot="tabs-trigger"][data-state="active"]');
      assert.ok(active, 'no active settings tab');
      assert.match(active.textContent ?? '', /Status/);
    });
  });
});

describe('Processes page', () => {
  test('renders live process rows from the socket, with state', async () => {
    goTo('/processes');
    render(<App />);
    const socket = await awaitSocket();

    await act(async () => {
      socket.accept();
      socket.send(frameAt('2026-01-01T00:00:00Z'));
    });

    // Populated over the WebSocket — the case that used to be permanently empty.
    assert.ok(screen.getByText('chrome'), 'process row should render');
    assert.ok(screen.getByText('4242'), 'pid should render');
    // Process.state was parsed, normalized and unit-tested but never displayed.
    assert.ok(screen.getByText('Running'), 'process state should render');
  });

  test('filters rows by the search box', async () => {
    goTo('/processes');
    render(<App />);
    const socket = await awaitSocket();

    await act(async () => {
      socket.accept();
      socket.send(frameAt('2026-01-01T00:00:00Z'));
    });

    // fireEvent.change goes through React's native value-setter tracker;
    // assigning .value directly is ignored by controlled inputs.
    const search = screen.getByLabelText('Filter processes');
    await act(async () => {
      fireEvent.change(search, { target: { value: 'nothing-matches-this' } });
    });

    assert.ok(screen.getByText(/Nothing matches/), 'should report an empty filter result');
  });
});
