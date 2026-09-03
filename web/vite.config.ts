import path from 'path';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig(() => {
    return {
      server: {
        port: 3000,
        host: '0.0.0.0',
        proxy: {
          '/api': 'http://localhost:8080',
          '/health': 'http://localhost:8080',
          '/ws': { target: 'ws://localhost:8080', ws: true },
        },
      },
      plugins: [
        react(),
        tailwindcss(),
      ],
      build: {
        rollupOptions: {
          output: {
            manualChunks: {
              // Split the heavy, rarely-changing vendors so a UI edit does not
              // invalidate the whole bundle in a user's cache.
              charts: ['recharts'],
              motion: ['motion'],
              radix: [
                '@radix-ui/react-select',
                '@radix-ui/react-tooltip',
                '@radix-ui/react-dropdown-menu',
                '@radix-ui/react-dialog',
                '@radix-ui/react-scroll-area',
                '@radix-ui/react-tabs',
              ],
              react: ['react', 'react-dom', 'react-router-dom'],
            },
          },
        },
      },
      resolve: {
        alias: {
          '@': path.resolve(__dirname, '.'),
        }
      }
    };
});
