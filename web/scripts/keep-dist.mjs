#!/usr/bin/env node
// Restores web/dist/.gitkeep after a build.
//
// web/embed.go embeds web/dist, and `//go:embed all:dist` is a compile-time
// directive: with no matching files the Go build fails outright with
// "pattern all:dist: no matching files found". A tracked placeholder keeps a
// clean checkout compiling before `npm run build` has ever run — which is
// exactly what CI's Go job does, since it never runs npm.
//
// Vite empties outDir on every build, so the placeholder has to be put back
// afterwards or the working tree shows it deleted after each `make web`.

import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const distDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'dist');

mkdirSync(distDir, { recursive: true });
writeFileSync(
  path.join(distDir, '.gitkeep'),
  [
    'Keeps web/dist present so `//go:embed all:dist` in web/embed.go compiles',
    'on a clean checkout, before the dashboard has ever been built.',
    '',
    'Recreated by scripts/keep-dist.mjs after every build, because Vite empties',
    'this directory. The built assets themselves stay untracked.',
    '',
  ].join('\n'),
);
