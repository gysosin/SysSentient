import React from 'react';
import { useNavigate } from 'react-router-dom';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';

/** Section order matches the navigation, so 1-6 map to what the eye already sees. */
const ROUTES = ['/', '/processes', '/logs', '/insights', '/alerts', '/settings'];

export const SHORTCUTS: { keys: string; action: string }[] = [
  { keys: '1 – 6', action: 'Jump to a section' },
  { keys: 'Space', action: 'Freeze / resume the live stream' },
  { keys: 'T', action: 'Switch between dark and light' },
  { keys: '?', action: 'Show this list' },
  { keys: 'Esc', action: 'Close a dialog' },
];

interface Props {
  /** Incremented by the header button to open the dialog. */
  openSignal?: number;
  onToggleFreeze: () => void;
  onToggleTheme: () => void;
}

/**
 * Global keyboard control for the console.
 *
 * "Fully keyboard operable" is a stated constraint, and administrators live on
 * keyboards — during an incident, reaching for a mouse to freeze a scrolling
 * process table is exactly the wrong ergonomics.
 */
export function ConsoleShortcuts({ openSignal = 0, onToggleFreeze, onToggleTheme }: Props) {
  const navigate = useNavigate();
  const [open, setOpen] = React.useState(false);

  React.useEffect(() => {
    if (openSignal > 0) setOpen(true);
  }, [openSignal]);

  React.useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      // Never hijack a key while someone is typing, and never fight a browser
      // or OS chord.
      const target = event.target as HTMLElement | null;
      if (
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        target?.isContentEditable ||
        ['INPUT', 'TEXTAREA', 'SELECT'].includes(target?.tagName ?? '')
      ) {
        return;
      }

      if (event.key >= '1' && event.key <= '6') {
        navigate(ROUTES[Number(event.key) - 1]);
        return;
      }

      switch (event.key) {
        case ' ':
          // Space would otherwise scroll the page.
          event.preventDefault();
          onToggleFreeze();
          break;
        case 't':
        case 'T':
          onToggleTheme();
          break;
        case '?':
          setOpen((prev) => !prev);
          break;
        default:
          break;
      }
    };

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [navigate, onToggleFreeze, onToggleTheme]);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>
            Available anywhere except while typing into a field.
          </DialogDescription>
        </DialogHeader>

        <dl className="divide-line divide-y">
          {SHORTCUTS.map((shortcut) => (
            <div key={shortcut.keys} className="flex items-center justify-between gap-4 py-2.5">
              <dt className="text-muted-foreground text-[13px]">{shortcut.action}</dt>
              <dd>
                <kbd className="border-line bg-panel-strong text-foreground rounded border px-2 py-1 font-mono text-[11px]">
                  {shortcut.keys}
                </kbd>
              </dd>
            </div>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  );
}

export default ConsoleShortcuts;
