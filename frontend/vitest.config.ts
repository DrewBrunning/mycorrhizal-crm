import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: './src/setupTests.ts',
    // Playwright E2E tests live in e2e/ and are run separately. The
    // root-level vite.config.test.ts (build-tooling config tests, node env)
    // is included explicitly because it lives outside src/. scripts/*.test.mjs
    // covers build-tooling scripts (the bundle-size budget, issue #556) the
    // same way -- outside src/, node env via the file's own pragma.
    include: ['src/**/*.{test,spec}.{ts,tsx}', 'viteConfig.test.ts', 'scripts/*.test.mjs'],
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
    // Vitest's default 5s test timeout is too tight for MUI dialog tests
    // under v8 coverage instrumentation on loaded CI runners (see the
    // 13-click deselection test in ExportFieldPickerDialog.test.tsx, which
    // overrides this per-test). 10s absorbs normal CI variance while still
    // catching genuine hangs reasonably fast.
    testTimeout: 10000,
    reporters: process.env.CI
      ? ['default', ['junit', { outputFile: './junit.xml' }], 'github-actions']
      : 'default',
    // Issue #251/#267: the project-wide number stays informational; the hard
    // gate is the diff-based codecov/patch status, which reads this lcov
    // output (target: 100% on changed lines). `text` for the CI log, `html`
    // for a browsable artifact, `lcov` for Codecov.
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      reporter: ['text', 'html', 'lcov'],
      reportsDirectory: './coverage',
    },
  },
});
