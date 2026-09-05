import { act, renderHook } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';

import { usePageVisible } from './usePageVisible';

function setVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

afterEach(() => {
  setVisibility('visible');
  vi.restoreAllMocks();
});

test('reports visible on a freshly loaded page', () => {
  const { result } = renderHook(() => usePageVisible());
  expect(result.current).toBe(true);
});

test('flips when the tab is hidden and again when it is shown', () => {
  const { result } = renderHook(() => usePageVisible());

  act(() => setVisibility('hidden'));
  expect(result.current).toBe(false);

  act(() => setVisibility('visible'));
  expect(result.current).toBe(true);
});

test('stops listening once unmounted', () => {
  const removed = vi.spyOn(document, 'removeEventListener');
  const { unmount } = renderHook(() => usePageVisible());
  unmount();

  // A listener left behind would keep firing state updates into an unmounted
  // hook on every tab switch for the life of the page.
  expect(removed).toHaveBeenCalledWith('visibilitychange', expect.any(Function));
});
