import path from 'path';
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Kept separate from vite.config.ts so the production build never pulls in test
// plugins. Note we deliberately do NOT load the tailwindcss plugin here: CSS is
// asserted against the real built bundle (see styles.build.test.ts), not against
// a test-time transform.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, '.') },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
    // Recursive by default — the previous `node --test .tmp-tests/**/*.test.js`
    // harness expanded `**` as a single `*` under sh, so any test at the web/
    // root was silently skipped with a green exit code.
    include: ['**/*.test.{ts,tsx}'],
    exclude: ['node_modules/**', 'dist/**', '.tmp-tests/**'],
    restoreMocks: true,
    unstubEnvs: true,
    unstubGlobals: true,
  },
});
