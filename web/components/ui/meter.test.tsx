import { render } from '@testing-library/react';
import { expect, test } from 'vitest';

import { Meter } from './motion-primitives';

/** The transform motion applied, or 'none' when there is nothing to translate. */
function fillTransform(value: number): { transform: string; className: string } {
  const { container } = render(<Meter value={value} />);
  const fill = container.querySelector('[role="meter"] > div') as HTMLElement;
  return { transform: fill.style.transform || 'none', className: fill.className };
}

/**
 * The bar is full width and slid left by the remainder, so the track clips it.
 * A 45% meter therefore sits at translateX(-55%) — the maths is inverted
 * relative to the old width animation, and getting it backwards would render
 * every bar as its own complement without failing anything else.
 */
test('a partially filled meter is slid left by the remainder', () => {
  const { transform, className } = fillTransform(45);
  expect(transform).toContain('translateX(-55%)');
  // Full width: the track does the clipping, so the rounded cap stays at the
  // fill point rather than being squashed by a scale.
  expect(className).toContain('w-full');
});

test('an empty meter is slid entirely out of its track', () => {
  expect(fillTransform(0).transform).toContain('translateX(-100%)');
});

test('a full meter needs no transform at all', () => {
  // motion omits a zero translate, which is what a completely filled bar is.
  expect(fillTransform(100).transform).toBe('none');
});

test('a meter clamps values outside 0-100 rather than overflowing its track', () => {
  expect(fillTransform(140).transform).toBe('none');
  expect(fillTransform(-20).transform).toContain('translateX(-100%)');
});

test('the meter still reports its value to assistive technology', () => {
  const { container } = render(<Meter value={45} label="CPU" />);
  const meter = container.querySelector('[role="meter"]') as HTMLElement;

  // The visual change must not quietly drop the accessible value.
  expect(meter.getAttribute('aria-valuenow')).toBe('45');
  expect(meter.getAttribute('aria-label')).toBe('CPU');
});
