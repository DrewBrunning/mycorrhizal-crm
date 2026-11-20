import { existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import react from '@vitejs/plugin-react';
import { playwright } from '@vitest/browser-playwright';
import { defineConfig } from 'vitest/config';

// Real-browser component tier (issue #2539). Same project as the jsdom suite
// but with a real Chromium layout engine: media queries, ResizeObserver /
// IntersectionObserver, scrolling and image measurement behave for real. Use it
// only for behaviour jsdom cannot prove (see docs/development/testing.md); the
// jsdom suite stays the default for everything else.
//
// MUST stay identical to findTestdataDir in vitest.config.ts (and to its
// setupFiles below). Duplicated deliberately: a local import without a file
// extension trips Vite's future-native-config-loader warning on every run.
function findTestdataDir(): string {
  for (let dir = import.meta.dirname; ; dir = dirname(dir)) {
    const candidate = join(dir, 'testdata');
    if (existsSync(join(candidate, 'contract-fixtures'))) return candidate;
    if (dirname(dir) === dir)
      throw new Error('vitest.browser.config.ts: repo-root testdata/ not found');
  }
}

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@testdata': findTestdataDir() },
  },
  test: {
    include: ['src/**/*.browser.test.tsx'],
    setupFiles: './src/setupTests.ts',
    browser: {
      enabled: true,
      provider: playwright(),
      instances: [{ browser: 'chromium' }],
      headless: true,
    },
    // A real scroll/measure pass is slower than a jsdom render but still an
    // order of magnitude below a Playwright e2e spec.
    testTimeout: 20000,
  },
});
