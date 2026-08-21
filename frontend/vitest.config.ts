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
    // Issue #268: CI-only retry so a flaky component test is retried once
    // instead of failing the required check. The github-actions reporter
    // writes anything that only passed on retry to the job summary ("Flaky
    // Tests"), and the junit reporter emits junit.xml for the
    // dorny/test-reporter check run (test-report.yml).
    retry: process.env.CI ? 1 : 0,
    reporters: process.env.CI
      ? [
          'default',
          ['junit', { outputFile: './junit.xml' }],
          'github-actions',
        ]
      : 'default',
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
