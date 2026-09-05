import { useEffect, useState } from 'react';

/**
 * Whether anyone can see this tab.
 *
 * `document` is absent under some test runners and during SSR; treating that
 * as visible keeps every consumer's timers running in those environments, which
 * is the behaviour they had before this hook existed.
 */
function readVisible(): boolean {
  return typeof document === 'undefined' || document.visibilityState !== 'hidden';
}

/**
 * Tracks page visibility so periodic work can stop while the tab is hidden.
 *
 * Nothing in the dashboard checked this. Chrome throttles timers in a
 * background tab but does not throttle WebSocket messages, so a hidden tab kept
 * receiving a frame every two seconds and re-rendering the whole application
 * for it — 43–76 DOM mutations a second with nobody looking, which is what made
 * Chrome flag the tab as "using extra resources". One hook, one source of
 * truth, so no consumer reimplements the check differently.
 */
export function usePageVisible(): boolean {
  const [visible, setVisible] = useState(readVisible);

  useEffect(() => {
    if (typeof document === 'undefined') return;
    const onChange = () => setVisible(readVisible());
    document.addEventListener('visibilitychange', onChange);
    // The state may have changed between the initial read and the subscribe.
    onChange();
    return () => document.removeEventListener('visibilitychange', onChange);
  }, []);

  return visible;
}
