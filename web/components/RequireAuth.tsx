import React from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';

import { useAuth } from '../hooks/useAuth';
import { Skeleton } from './ui/skeleton';

/**
 * Route guard. Renders nested routes only once a session is confirmed, so the
 * dashboard never mounts its socket and pollers against a 401.
 */
export const RequireAuth: React.FC = () => {
  const { status } = useAuth();
  const location = useLocation();

  if (status === 'loading') {
    return (
      <div
        role="status"
        aria-live="polite"
        className="gridbg flex min-h-screen items-center justify-center p-6"
      >
        <Skeleton className="h-8 w-48" />
        <span className="sr-only">Checking your session…</span>
      </div>
    );
  }
  if (status === 'setup') return <Navigate to="/setup" replace />;
  if (status === 'anon') {
    // Remember where they were headed so sign-in returns them there.
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  return <Outlet />;
};
