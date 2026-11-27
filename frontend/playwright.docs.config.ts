import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for the documentation screenshots (issue #2244).
 *
 * Deliberately a SEPARATE config from playwright.config.ts, like the
 * service-worker suite: this is a generator, not a test suite. It runs against
 * the pen-test/demo stack (docker-compose.pentest.yml, `demo` profile: one user
 * with 35 contacts, clean names) and rewrites docs/assets/screenshots/*.png.
 * It is never part of CI. See docs/development/documentation-site.md.
 *
 * Run:  scripts/docs-screenshots.sh
 */
export default defineConfig({
  testDir: './e2e/docs-screenshots',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.DOCS_BASE_URL ?? 'http://localhost:7300',
    ...devices['Desktop Chrome'],
    viewport: { width: 1280, height: 800 },
    colorScheme: 'light',
    locale: 'en-US',
    timezoneId: 'UTC',
  },
});
