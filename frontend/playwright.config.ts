import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for Mycorrhizal CRM E2E tests.
 *
 * Run tests with: npx playwright test
 * Run with UI:    npx playwright test --ui
 * Run specific:   npx playwright test auth.spec.ts
 */
export default defineConfig({
  testDir: './e2e',

  // Issue #476 (WEB-02): the service-worker upgrade suite lives under
  // e2e/sw-upgrade/ but runs under its OWN config (playwright.sw.config.ts)
  // against its own two-build harness on :7300 -- never under this config,
  // which runs against the single-build docker stack. Excluding it here is
  // what keeps `npx playwright test` from accidentally trying to run those
  // specs against the wrong server. The project-level testIgnore below
  // repeats the pattern for the authenticated project.
  testIgnore: [/sw-upgrade/],

  fullyParallel: true,

  // Fail the build on CI if you accidentally left test.only in the source code
  forbidOnly: !!process.env.CI,

  // Retry on CI only. Issue #974: the nightly `schedule` run (this repo's
  // e2e-tests.yml runs its suite jobs unconditionally on schedule) gets 0
  // retries via the default GITHUB_EVENT_NAME Actions env var, so a flaky
  // spec fails hard at least once a day instead of staying green forever
  // behind the retry. PR/push keep retrying once.
  retries: process.env.CI ? (process.env.GITHUB_EVENT_NAME === 'schedule' ? 0 : 1) : 0,

  // Issue #1480: every worker authenticates as its OWN account (a worker-scoped
  // fixture in e2e/fixtures.ts registers + seeds it through the API), so
  // account-level settings a test mutates never reach another worker. That
  // removed the shared-user race that forced the cross-process settings lock and
  // the 1-worker pin on the release run (issue #1177), so CI runs 4 workers. The
  // DB file is still shared (SQLite has one writer) but the rows are disjoint.
  // PLAYWRIGHT_WORKERS stays as a manual override for local bisecting.
  workers: process.env.PLAYWRIGHT_WORKERS
    ? Number(process.env.PLAYWRIGHT_WORKERS)
    : process.env.CI
      ? 4
      : undefined,

  // Reporter to use. The JSON reporter (issue #1488) is the flake ledger's
  // input: per-test status incl. "flaky" (failed then passed on retry). It is
  // advisory data only -- nothing fails on it. The workflow uploads it as the
  // `flake-playwright-<job>` artifact (cmd/flakeledger parses it).
  reporter: [
    ['html', { open: 'never' }],
    ['list'],
    ['json', { outputFile: 'playwright-results/results.json' }],
  ],

  // Shared settings for all projects
  use: {
    baseURL: 'http://localhost:7300',

    // Collect trace when retrying the failed test
    trace: 'on-first-retry',

    screenshot: 'only-on-failure',
    video: 'on-first-retry',
  },

  projects: [
    // Authenticates the shared (auto-admin) `testuser` once and writes
    // playwright/.auth/user.json.
    {
      name: 'setup',
      testMatch: /.*\.setup\.ts/,
    },
    // Specs get their auth state from the per-worker `storageState` fixture
    // override in e2e/fixtures.ts -- deliberately NOT set here, since a config
    // value is one file for every worker. Specs that need a logged-out state
    // (e.g. auth.spec.ts) opt out via test.use({ storageState: LOGGED_OUT });
    // admin-only / seeded-exact specs opt into the shared user with
    // test.use({ sharedUser: true }). The `setup` project still logs the shared
    // `testuser` in, for those and for admin cleanup.
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
      dependencies: ['setup'],
      testIgnore: [/.*\.setup\.ts/, /sw-upgrade/, /webkitSmoke/],
    },
    // Issue #992: the only WebKit coverage in this repo. Deliberately its own
    // project rather than folded into 'chromium' -- webkitSmoke.spec.ts logs
    // in through the UI itself instead of depending on 'setup'/storageState,
    // so this engine's single spec can run standalone without dragging the
    // rest of the (chromium-only-verified) suite onto an engine it was never
    // written against. See that file's header for what it does and does not
    // prove.
    {
      name: 'webkit',
      use: {
        ...devices['Desktop Safari'],
      },
      testMatch: /webkitSmoke\.spec\.ts/,
    },
  ],

  // Global setup for test data seeding
  globalSetup: './e2e/global-setup.ts',

  timeout: 30000,
  expect: {
    timeout: 5000,
    // Issue #1177: antialiasing/subpixel rendering is nondeterministic under
    // CI load — RC2 failed three screenshots on a 5-pixel diff. A small
    // absolute tolerance absorbs that noise; a real layout change moves far
    // more than this, so a regression is still caught.
    toHaveScreenshot: {
      maxDiffPixels: 50,
    },
  },
});
