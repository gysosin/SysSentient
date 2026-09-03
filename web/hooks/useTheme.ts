import * as React from 'react';

export type Theme = 'dark' | 'light';

const STORAGE_KEY = 'syssentient.theme';

/**
 * Dark by default, because this console is mostly seen on a wall display or a
 * second monitor at night. The light tokens exist and are fully designed, but
 * nothing reached them until there was a switch.
 *
 * The choice is remembered per browser. Storage is wrapped because a private
 * window, cleared site data, or a locked-down browser makes it throw, and a
 * theme preference is never worth breaking the page over.
 */
export function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = React.useState<Theme>(() => {
    try {
      const stored = window.localStorage.getItem(STORAGE_KEY);
      if (stored === 'light' || stored === 'dark') return stored;
    } catch {
      // Ignore: fall through to the default.
    }
    return 'dark';
  });

  React.useEffect(() => {
    // The token blocks key off `.light`; its absence is the dark theme.
    document.documentElement.classList.toggle('light', theme === 'light');
    try {
      window.localStorage.setItem(STORAGE_KEY, theme);
    } catch {
      // Ignore: the theme still applies for this session.
    }
  }, [theme]);

  const toggle = React.useCallback(() => {
    setTheme((prev) => (prev === 'dark' ? 'light' : 'dark'));
  }, []);

  return [theme, toggle];
}
