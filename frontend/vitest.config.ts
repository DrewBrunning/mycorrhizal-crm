import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: './src/setupTests.ts',
    // Playwright E2E tests live in e2e/ and are run separately. The
    // root-level vite.config.test.ts (build-tooling config tests, node env)
    // is included explicitly because it lives outside src/.
    include: ['src/**/*.{test,spec}.{ts,tsx}', 'viteConfig.test.ts'],
    // Node >= 22 ships its own experimental localStorage global, which is
    // undefined without --localstorage-file and shadows jsdom's working one
    pool: 'forks',
    poolOptions: {
      forks: {
        execArgv: ['--no-experimental-webstorage'],
      },
    },
    // Issue #251: visibility only, no thresholds/gate — a separate ticket
    // tracks enforcing coverage. `text` for the CI log, `html` for a
    // browsable artifact, `lcov` for third-party tooling that wants it.
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      reporter: ['text', 'html', 'lcov'],
      reportsDirectory: './coverage',
    },
  },
});
