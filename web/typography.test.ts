import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, test } from 'vitest';

// Guards the readable-type floor.
//
// The console has now twice drifted into unreadable type. tailwind.config.js
// once carried a comment calling 15 hardcoded `text-[10px]` usages "below any
// legibility floor"; by the time this test was written there were 72 arbitrary
// sizes between 9px and 13px, four of them at 9px. Log timestamps in
// particular were unreadable.
//
// A scale in index.css is not enough on its own, because nothing stops the
// next component from reaching for an arbitrary value again. This does.

const here = path.dirname(fileURLToPath(import.meta.url));
const SEARCH_DIRS = ['components', 'pages', 'hooks'];
const MIN_PX = 12;

function sourceFiles(dir: string): string[] {
  const abs = path.join(here, dir);
  let entries: string[];
  try {
    entries = readdirSync(abs);
  } catch {
    return [];
  }
  return entries.flatMap((entry) => {
    const full = path.join(abs, entry);
    if (statSync(full).isDirectory()) return sourceFiles(path.join(dir, entry));
    return /\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry) ? [path.join(dir, entry)] : [];
  });
}

describe('type stays readable', () => {
  test(`no arbitrary font size below ${MIN_PX}px`, () => {
    const offenders: string[] = [];

    for (const rel of SEARCH_DIRS.flatMap(sourceFiles)) {
      const contents = readFileSync(path.join(here, rel), 'utf8');
      contents.split('\n').forEach((line, i) => {
        for (const match of line.matchAll(/text-\[(\d+)px\]/g)) {
          const px = Number(match[1]);
          if (px < MIN_PX) offenders.push(`${rel}:${i + 1}  ${match[0]}`);
        }
      });
    }

    assert.deepEqual(
      offenders,
      [],
      `font sizes below ${MIN_PX}px found. Use the scale in index.css ` +
        `(text-2xs is the ${MIN_PX}px floor) rather than an arbitrary value:\n  ` +
        offenders.join('\n  '),
    );
  });

  test('the scale the floor depends on is actually defined', () => {
    // A test asserting "nothing below 12px" passes trivially if the scale it
    // points people at does not exist, so assert both halves.
    const css = readFileSync(path.join(here, 'index.css'), 'utf8');
    for (const token of ['--text-2xs:', '--text-xs:', '--text-sm:']) {
      assert.ok(css.includes(token), `type scale is missing ${token}`);
    }
  });

  test('the layout is not capped to a fixed pixel width', () => {
    // The shell used to cap at 1600px, which left roughly a third of a 2560px
    // display unused on a product whose job is dense information.
    const shell = readFileSync(path.join(here, 'components/AppShell.tsx'), 'utf8');
    const caps = [...shell.matchAll(/max-w-\[(\d+)px\]/g)].map((m) => m[0]);
    assert.deepEqual(caps, [], `AppShell should be fluid, found: ${caps.join(', ')}`);
  });
});
