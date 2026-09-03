import React, { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { useAuth } from '../hooks/useAuth';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';

const Login: React.FC = () => {
  const { status, signIn } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from || '/';

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const errorRef = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    if (status === 'authed') navigate(from, { replace: true });
    if (status === 'setup') navigate('/setup', { replace: true });
  }, [status, navigate, from]);

  // Move focus to the message so a screen reader user is not left guessing
  // why the form did not submit.
  useEffect(() => {
    if (error) errorRef.current?.focus();
  }, [error]);

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setPending(true);
    setError(null);
    try {
      await signIn(email, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign-in failed');
    } finally {
      setPending(false);
    }
  };

  const describedBy = error ? 'login-error' : undefined;

  return (
    <main className="gridbg flex min-h-screen items-center justify-center p-4">
      <Card className="elevated w-full max-w-sm">
        <CardHeader className="flex-col items-start gap-0">
          {/* The same monogram lockup the console header carries, so the first
              screen anyone sees already establishes the product's voice. */}
          <div className="flex items-center gap-3">
            <span className="border-brand/40 bg-brand-soft text-brand font-display grid size-9 place-items-center rounded-md border text-lg font-bold">
              SS
            </span>
            <span className="font-display text-[15px] font-semibold tracking-[0.18em] uppercase">
              SysSentient
            </span>
          </div>
          <p className="text-brand mt-8 text-[10px] font-semibold tracking-[0.22em] uppercase">
            Operator access
          </p>
          <CardTitle className="mt-2 text-4xl tracking-tight">Sign in.</CardTitle>
          <CardDescription className="mt-3 text-sm">Monitoring console</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} noValidate className="grid gap-4" aria-busy={pending}>
            <div className="grid gap-1.5">
              <Label htmlFor="login-email">Email</Label>
              <Input
                id="login-email"
                type="email"
                autoComplete="email"
                required
                autoFocus
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                aria-invalid={error ? true : undefined}
                aria-describedby={describedBy}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="login-password">Password</Label>
              <Input
                id="login-password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                aria-invalid={error ? true : undefined}
                aria-describedby={describedBy}
              />
            </div>
            {error && (
              <p
                id="login-error"
                ref={errorRef}
                tabIndex={-1}
                role="alert"
                className="text-crit text-sm"
              >
                {error}
              </p>
            )}
            <Button type="submit" disabled={pending} className="mt-1">
              {pending ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
};

export default Login;
