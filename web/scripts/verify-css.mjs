#!/usr/bin/env node
// Proves the Tailwind config actually reached the compiled bundle.
//
// styles.config.test.ts asserts the @config directive is *declared*; this
// asserts it had an *effect*. Both are needed: a typo in the config path, a
// Tailwind major bump, or someone "cleaning up" index.css would leave the
// directive present but the utilities missing, and the dashboard would silently
// revert to gray-on-black with an invisible AI panel.
//
// Run after `npm run build`.

import { readFileSync, readdirSync, existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const webDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const assetsDir = path.join(webDir, 'dist', 'assets');

if (!existsSync(assetsDir)) {
  console.error(`verify-css: ${assetsDir} not found — run \`npm run build\` first.`);
  process.exit(1);
}

const cssFiles = readdirSync(assetsDir).filter((f) => f.endsWith('.css'));
if (cssFiles.length === 0) {
  console.error('verify-css: no .css emitted into dist/assets.');
  process.exit(1);
}

const css = cssFiles.map((f) => readFileSync(path.join(assetsDir, f), 'utf8')).join('\n');

// Sampled from the classes actually used across App.tsx and the components.
const required = [
  // shadcn semantic tokens: if @theme mapping breaks, every component loses
  // its colour and the UI silently reverts to unstyled boxes.
  { needle: 'bg-card', why: 'every panel surface' },
  { needle: 'text-muted-foreground', why: 'secondary text throughout' },
  { needle: 'bg-primary', why: 'primary buttons and active nav' },
  { needle: 'text-crit', why: 'critical values and ERROR log lines' },
  { needle: 'text-warn', why: 'warning thresholds' },
  { needle: 'text-ok', why: 'healthy values' },
  { needle: '--radius', why: 'the shared corner radius scale' },
  { needle: '@keyframes fadeIn', why: 'AI panel entrance' },
  { needle: 'elevated', why: 'panel depth; a flat card reads as unstyled' },

  // Console redesign tokens. These carry the shell and every screen, so a
  // mapping failure here is the whole design vanishing, not a detail.
  { needle: 'text-brand', why: 'brand mark, eyebrows and the live indicator' },
  { needle: 'bg-panel', why: 'every section surface' },
  { needle: 'border-line', why: 'every divider and table rule' },
  { needle: 'text-mute', why: 'the supporting-text ramp' },
  { needle: 'gridbg', why: 'the ambient measurement grid behind the console' },
  { needle: 'live-pulse', why: 'the feed liveness dot' },
  { needle: 'scanline', why: 'the hero sweep' },

  // Bundled fonts. A missing @font-face does not error — it falls back to a
  // system face, so the typography silently changes on air-gapped hosts.
  { needle: '@font-face', why: 'self-hosted Sora / Manrope / JetBrains Mono' },
  { needle: 'Sora Variable', why: 'the display face used by every heading' },
];

const missing = required.filter(({ needle }) => !css.includes(needle));

if (missing.length > 0) {
  console.error('verify-css: FAILED — the Tailwind config is not reaching the bundle.\n');
  for (const { needle, why } of missing) {
    console.error(`  missing: ${needle}\n     used for: ${why}`);
  }
  console.error('\nCheck the `@config` directive at the top of web/index.css.');
  process.exit(1);
}

console.log(`verify-css: OK — all ${required.length} required style tokens present in ${cssFiles.join(', ')}`);
