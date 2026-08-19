import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    // InjectManifest strategy to match CRA's Workbox setup: the service worker
    // is a hand-written file (src/service-worker.ts) whose precache manifest is
    // injected at build time. Registration stays manual via
    // src/serviceWorkerRegistration.ts (index.tsx calls register()), so the
    // plugin must not inject its own registerSW() call into index.html.
    VitePWA({
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'service-worker.ts',
      injectRegister: false,
      // The web manifest lives in public/manifest.json (dark/light themed
      // icons) and is linked from index.html directly -- let the plugin not
      // generate a competing one.
      manifest: false,
      // The app legitimately ships the entire @mdi/js icon set (2.7MB): the
      // free-text link-field-type icon input (T43) does a dynamic lookup
      // against the namespace import, so it cannot be tree-shaken. Workbox's
      // default 2MiB precache limit would fail the build on that one chunk
      // (vite-plugin-pwa defaults throwMaximumFileSizeToCacheInBytes to true,
      // unlike workbox-build's warn-only default that CRA used). Raise it to
      // cover the largest chunk with headroom rather than drop it from the
      // precache, which would break offline/Web Push.
      injectManifest: {
        maximumFileSizeToCacheInBytes: 4 * 1024 * 1024,
      },
    }),
  ],
  server: {
    // Matches .claude/launch.json's frontend-dev config and the e2e suite's
    // hardcoded base URL (frontend/e2e/global-setup.ts).
    port: 7300,
  },
  build: {
    // Keep CRA's output directory so the Dockerfile COPY paths and .gitignore
    // entries stay unchanged.
    outDir: 'build',
    rollupOptions: {
      output: {
        // CRA (webpack) split vendor code out of the app bundle; Vite's default
        // single chunk is one ~4MB file, which both defeats HTTP caching and
        // blows past Workbox's 2MiB precache limit (vite-plugin-pwa fails the
        // build on it). Restore a coarse vendor split. Vite 8's Rolldown
        // bundler only accepts a function here (the Rollup object form that
        // mapped a chunk to bare module names was removed), so match resolved
        // package names against the same groups.
        manualChunks(id) {
          if (!id.includes('/node_modules/')) return undefined;
          const rest = id.slice(id.indexOf('/node_modules/') + '/node_modules/'.length);
          const pkg = rest.startsWith('@')
            ? rest.split('/').slice(0, 2).join('/')
            : rest.split('/')[0];
          for (const [chunk, packages] of VENDOR_CHUNKS) {
            if (packages.includes(pkg)) return chunk;
          }
          return undefined;
        },
      },
    },
  },
});

const VENDOR_CHUNKS: ReadonlyArray<readonly [string, readonly string[]]> = [
  ['react-vendor', ['react', 'react-dom', 'react-router']],
  ['mui-core', ['@mui/material', '@mui/lab', '@emotion/react', '@emotion/styled']],
  ['mui-icons', ['@mui/icons-material']],
  ['mdi', ['@mdi/react', '@mdi/js']],
  ['i18n-vendor', ['i18next', 'react-i18next', 'i18next-browser-languagedetector']],
  ['graph-vendor', ['react-force-graph-2d', 'd3-force']],
];
