import React, { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { useAuth } from '../hooks/useAuth';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';

const MIN_PASSWORD_LENGTH = 12;

const Setup: React.FC = () => {
  const { status, finishSetup } = useAuth();
  const navigate = useNavigate();

  const [token, setToken] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const errorRef = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    if (status === 'authed') navigate('/', { replace: true });
    // Setup already done by someone else — send them to sign in instead.
    if (status === 'anon') navigate('/login', { replace: true });
  }, [status, navigate]);

  useEffect(() => {
    if (error) errorRef.current?.focus();
  }, [error]);

  const mismatch = confirm.length > 0 && password !== confirm;
  const tooShort = password.length > 0 && password.length < MIN_PASSWORD_LENGTH;

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    // Check locally first so a typo costs a round trip, not the one-time token.
    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`);
      return;
    }
    if (password !== confirm) {
      setError('The two passwords do not match.');
      return;
    }
    setPending(true);
    setError(null);
    try {
      await finishSetup(token.trim(), email, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed');
    } finally {
      setPending(false);
    }
  };

  const describe = (...ids: (string | false | null | undefined)[]) => {
    const list = ids.filter(Boolean).join(' ');
    return list || undefined;
  };

  return (
    <main className="gridbg flex min-h-screen items-center justify-center p-4">
      <Card className="elevated w-full max-w-md">
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
            First run
          </p>
          <CardTitle className="mt-2 text-4xl tracking-tight">Set up.</CardTitle>
          <CardDescription className="mt-3 text-sm">
            This account owns the console. You can add more users afterwards.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} noValidate className="grid gap-4" aria-busy={pending}>
            <div className="grid gap-1.5">
              <Label htmlFor="setup-token">Setup token</Label>
              <Input
                id="setup-token"
                autoComplete="off"
                spellCheck={false}
                required
                autoFocus
                value={token}
                onChange={(e) => setToken(e.target.value)}
                className="font-mono text-xs"
                aria-describedby={describe('setup-token-hint', error && 'setup-error')}
              />
              <p id="setup-token-hint" className="text-muted-foreground text-xs">
                Printed once in the daemon log at startup.
              </p>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="setup-email">Email</Label>
              <Input
                id="setup-email"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                aria-describedby={describe(error && 'setup-error')}
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="setup-password">Password</Label>
              <Input
                id="setup-password"
                type="password"
                autoComplete="new-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                aria-invalid={tooShort || undefined}
                aria-describedby={describe('setup-password-hint', error && 'setup-error')}
              />
              <p id="setup-password-hint" className="text-muted-foreground text-xs">
                At least {MIN_PASSWORD_LENGTH} characters.
              </p>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="setup-confirm">Confirm password</Label>
              <Input
                id="setup-confirm"
                type="password"
                autoComplete="new-password"
                required
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                aria-invalid={mismatch || undefined}
                aria-describedby={describe(error && 'setup-error')}
              />
            </div>

            {error && (
              <p
                id="setup-error"
                ref={errorRef}
                tabIndex={-1}
                role="alert"
                className="text-crit text-sm"
              >
                {error}
              </p>
            )}

            <Button type="submit" disabled={pending} className="mt-1">
              {pending ? 'Creating…' : 'Create admin'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
};

export default Setup;
