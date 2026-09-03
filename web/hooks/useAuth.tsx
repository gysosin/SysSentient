import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import {
  AuthUser,
  completeSetup as apiCompleteSetup,
  fetchMe,
  fetchSetupStatus,
  login as apiLogin,
  logout as apiLogout,
  onUnauthorized,
} from '../services/api';

export type AuthStatus = 'loading' | 'setup' | 'anon' | 'authed';

interface AuthContextValue {
  status: AuthStatus;
  user: AuthUser | null;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  finishSetup: (token: string, email: string, password: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

/**
 * Owns the session state machine: loading → setup | anon | authed.
 *
 * One probe of /api/auth/me on mount decides the branch. A 401 from any later
 * data request drops back to `anon` through the api module's hook, so a
 * revoked or expired session surfaces immediately instead of leaving the
 * dashboard rendering stale numbers.
 */
export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [status, setStatus] = useState<AuthStatus>('loading');
  const [user, setUser] = useState<AuthUser | null>(null);

  useEffect(() => {
    onUnauthorized(() => {
      setUser(null);
      setStatus('anon');
    });
    return () => onUnauthorized(null);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const me = await fetchMe();
        if (cancelled) return;
        if (me) {
          setUser(me);
          setStatus('authed');
          return;
        }
        const needsSetup = await fetchSetupStatus();
        if (!cancelled) setStatus(needsSetup ? 'setup' : 'anon');
      } catch {
        // The daemon is unreachable. Fall through to the login page, which
        // surfaces the real error on submit, rather than trapping the user
        // behind a spinner that never resolves.
        if (!cancelled) setStatus('anon');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const signIn = useCallback(async (email: string, password: string) => {
    const me = await apiLogin(email, password);
    setUser(me);
    setStatus('authed');
  }, []);

  const signOut = useCallback(async () => {
    await apiLogout();
    setUser(null);
    setStatus('anon');
  }, []);

  const finishSetup = useCallback(async (token: string, email: string, password: string) => {
    const me = await apiCompleteSetup(token, email, password);
    setUser(me);
    setStatus('authed');
  }, []);

  const value = useMemo(
    () => ({ status, user, signIn, signOut, finishSetup }),
    [status, user, signIn, signOut, finishSetup],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
