import React from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';

import AppShell from './components/AppShell';
import ErrorBoundary from './components/ErrorBoundary';
import { RequireAuth } from './components/RequireAuth';
import { AuthProvider, useAuth } from './hooks/useAuth';
import { DashboardProvider, useDashboard } from './hooks/useDashboardData';
import { useTheme } from './hooks/useTheme';
import ConsoleShortcuts from './components/ConsoleShortcuts';
import { formatDuration } from './lib/utils';

import Overview from './pages/Overview';
import Processes from './pages/Processes';
import Hosts from './pages/Hosts';
import Logs from './pages/Logs';
import Insights from './pages/Insights';
import Alerts from './pages/Alerts';
import Settings from './pages/Settings';
import Login from './pages/Login';
import Setup from './pages/Setup';

/** Reads shared state so the shell can render feed status in the header. */
const Shell: React.FC = () => {
  const { feed, current, hosts, selectedHost, selectHost, firingAlerts, frozen, toggleFreeze } =
    useDashboard();
  const { user, signOut } = useAuth();
  const [theme, toggleTheme] = useTheme();

  // The help dialog is opened from two places — the header button and the "?"
  // key — so the open state lives here and the key handler owns the rest.
  const [shortcutsSignal, setShortcutsSignal] = React.useState(0);

  return (
    <>
      <ConsoleShortcuts
        openSignal={shortcutsSignal}
        onToggleFreeze={toggleFreeze}
        onToggleTheme={toggleTheme}
      />
      <AppShell
        feed={feed}
        hostname={current.hostname || 'unknown host'}
        uptimeLabel={formatDuration(current.uptimeSeconds)}
        hosts={hosts}
        selectedHost={selectedHost}
        onSelectHost={selectHost}
        firingAlerts={firingAlerts}
        user={user}
        onSignOut={() => void signOut()}
        frozen={frozen}
        onToggleFreeze={toggleFreeze}
        theme={theme}
        onToggleTheme={toggleTheme}
        onShowShortcuts={() => setShortcutsSignal((n) => n + 1)}
      />
    </>
  );
};

/**
 * The live data feed only starts once a session exists. Mounting it outside
 * RequireAuth would leave the socket and pollers spinning on 401s behind the
 * login page.
 */
const Console: React.FC = () => (
  <DashboardProvider>
    <Shell />
  </DashboardProvider>
);

const App: React.FC = () => (
  <ErrorBoundary>
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/setup" element={<Setup />} />
          <Route element={<RequireAuth />}>
            <Route element={<Console />}>
              <Route index element={<Overview />} />
              <Route path="processes" element={<Processes />} />
              <Route path="hosts" element={<Hosts />} />
              <Route path="logs" element={<Logs />} />
              <Route path="insights" element={<Insights />} />
              <Route path="alerts" element={<Alerts />} />
              <Route path="settings" element={<Settings />} />
            </Route>
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  </ErrorBoundary>
);

export default App;
