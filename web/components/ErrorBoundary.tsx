import React from 'react';
import { TriangleAlert } from 'lucide-react';

import { Button } from './ui/button';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';

interface Props {
  children: React.ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * Without this, a single render error blanks the whole dashboard with no
 * message and no way back — the app had no boundary of any kind.
 */
export default class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('Dashboard render error:', error, info.componentStack);
  }

  private handleReset = () => {
    this.setState({ error: null });
  };

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <div className="gridbg grid min-h-screen place-items-center p-6" role="alert">
        <Card className="border-crit/40 w-full max-w-lg">
          <CardHeader>
            <TriangleAlert className="text-crit size-4" />
            <CardTitle>The dashboard hit an error</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-muted-foreground text-sm">
              This is a bug in the interface, not necessarily in the daemon. Metrics collection is
              unaffected.
            </p>
            <pre className="bg-muted/60 text-muted-foreground overflow-x-auto rounded-md border p-3 font-mono text-xs">
              {error.message}
            </pre>
            <Button onClick={this.handleReset}>Try again</Button>
          </CardContent>
        </Card>
      </div>
    );
  }
}
