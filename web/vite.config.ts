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
            // Split the heavy, rarely-changing vendors so a UI edit does not
            // invalidate the whole bundle in a user's cache.
            //
            // Written as a function rather than the object form: Vite 8 builds
            // on rolldown, whose `manualChunks` only accepts
            // `(id) => chunkName`. The object map silently became a type error
            // rather than being ignored, which is the good outcome — a
            // misconfigured chunk split would otherwise just quietly stop
            // splitting.
            manualChunks(id: string) {
              if (!id.includes('node_modules')) return undefined;
              if (id.includes('node_modules/recharts')) return 'charts';
              if (id.includes('node_modules/motion')) return 'motion';
              if (id.includes('node_modules/@radix-ui')) return 'radix';
              if (
                /node_modules\/(react|react-dom|react-router|react-router-dom|scheduler)\//.test(id)
              ) {
                return 'react';
              }
              return undefined;
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
