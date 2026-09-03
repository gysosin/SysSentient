import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// React Testing Library auto-cleanup only registers when a global `afterEach`
// exists at import time; register it explicitly so component state never leaks
// between tests.
afterEach(() => {
  cleanup();
});

// jsdom implements neither of these, and recharts' ResponsiveContainer needs
// ResizeObserver on mount. Without them any test that renders a chart throws
// before a single assertion runs.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
}

if (!globalThis.matchMedia) {
  globalThis.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof globalThis.matchMedia;
}

// jsdom does not implement scrollIntoView; App auto-scrolls the log pane.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = function scrollIntoView() {};
}
