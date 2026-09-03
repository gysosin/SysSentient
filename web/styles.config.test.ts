import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, test } from 'vitest';

// Regression guard for the highest-impact defect in the dashboard.
//
// Tailwind v4 does NOT auto-load `tailwind.config.js`. Without an explicit
// `@config` directive, every custom utility in that file (the whole semantic
// palette) silently compiles to nothing — which is exactly what happened to
// the previous design system: 49 class usages became no-ops, the UI rendered
// as undesigned gray, and the AI panel's output was invisible.
//
// These assertions are cheap and run on every `npm test`. The companion
// build-output check (`npm run verify:css`) proves the classes actually reach
// the compiled bundle.

const here = path.dirname(fileURLToPath(import.meta.url));
const indexCss = readFileSync(path.join(here, 'index.css'), 'utf8');

describe('tailwind configuration is actually loaded', () => {
  test('index.css declares an @config directive', () => {
    assert.match(
      indexCss,
      /@config\s+["'][^"']+["']\s*;/,
      'index.css must declare `@config` — Tailwind v4 ignores tailwind.config.js without it',
    );
  });

  test('the @config directive points at a file that exists', () => {
    const match = indexCss.match(/@config\s+["']([^"']+)["']\s*;/);
    assert.ok(match, 'no @config directive found');
    const resolved = path.resolve(here, match[1]);
    assert.ok(
      existsSync(resolved),
      `@config points at "${match[1]}" which does not exist (resolved: ${resolved})`,
    );
  });

  test('keyframes referenced by components are defined', () => {
    // AIInsightPanel.tsx renders each recommended action with
    // `animate-[fadeIn_0.5s_ease-out_forwards] opacity-0`. If the fadeIn
    // keyframe is missing the animation never runs, `forwards` never applies,
    // and opacity:0 sticks permanently — the AI's output renders invisible.
    const config = readFileSync(path.join(here, 'tailwind.config.js'), 'utf8');
    const combined = `${indexCss}\n${config}`;

    assert.ok(combined.includes('fadeIn'), 'fadeIn must be defined');
    assert.match(
      indexCss,
      /@keyframes\s+fadeIn\b/,
      'fadeIn must be a real @keyframes block in index.css',
    );
  });

  test('the semantic token layer is defined', () => {
    // Components reference roles (--card, --primary, --muted-foreground), not
    // raw colours. If this block is lost the whole UI renders unstyled.
    //
    // --brand/--panel/--line/--mute/--melt came in with the console redesign
    // and now carry the shell, every panel surface and every divider; losing
    // one of them is not a cosmetic regression, it is an unstyled screen.
    const tokens = [
      '--background', '--card', '--primary', '--muted-foreground',
      '--ok', '--warn', '--crit',
      '--brand', '--brand-soft', '--panel', '--panel-strong',
      '--line', '--mute', '--melt',
    ];
    for (const token of tokens) {
      assert.ok(indexCss.includes(token), `semantic token ${token} is not defined`);
    }
    // And they must be mapped into Tailwind, or `bg-card` etc. do not exist.
    assert.match(indexCss, /@theme inline\s*\{/, 'tokens are not mapped into the Tailwind theme');
    // Every token needs a light value too, or the light theme inherits dark
    // surfaces and renders white-on-white.
    const light = indexCss.slice(indexCss.indexOf('.light {'), indexCss.indexOf('@theme inline'));
    for (const token of tokens) {
      assert.ok(light.includes(token), `${token} has no value in the .light theme`);
    }
  });

  test('the display face is registered as a Tailwind utility', () => {
    // Headings use `font-display`. Registering the family in @theme is what
    // creates that utility; without it every heading silently renders in the
    // body face and the design loses its voice with no error anywhere.
    assert.match(
      indexCss,
      /--font-display:/,
      '--font-display must be registered in @theme inline so `font-display` exists',
    );
  });

  test('fonts are bundled, never fetched from a CDN', () => {
    // This product ships to firewalled and air-gapped hosts. A remote font
    // link does not fail loudly — it falls back to a system face, so the
    // typography quietly changes on exactly the deployments we care about.
    const indexHtml = readFileSync(path.join(here, 'index.html'), 'utf8');
    assert.ok(
      !/fonts\.(googleapis|gstatic)\.com/.test(indexHtml),
      'index.html must not reference a font CDN — fonts are bundled via @fontsource-variable',
    );
    assert.match(
      indexCss,
      /@import\s+["']@fontsource-variable\//,
      'index.css must import the bundled font faces',
    );
  });

  test('the content glob excludes node_modules', () => {
    const config = readFileSync(path.join(here, 'tailwind.config.js'), 'utf8');
    // Once @config actually loads, an unbounded `./**/*` glob makes Tailwind
    // crawl node_modules on every build.
    assert.ok(
      /!\s*\.\/node_modules/.test(config) || /"!\.\/node_modules/.test(config),
      'tailwind.config.js content globs must exclude node_modules',
    );
  });
});
