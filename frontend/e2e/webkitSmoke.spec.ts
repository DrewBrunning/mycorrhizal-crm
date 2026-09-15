import { expect, test } from './fixtures';
import { TEST_USER } from './global-setup';

// Issue #992 (split from #917 finding G6): docs/development/supported-runtime-matrix.md's
// "Browsers" row and docs/supported-versions.md's "Web browser" row set the
// stated floor -- Chrome/Edge/Firefox >=111, Safari/iOS >=16.4 -- from Web
// Push support (frontend/src/pushSubscription.ts's browserSupportsPush()),
// but every browser job in this repo before this file drove Chromium or
// Firefox: this suite's own `chromium` project, e2e-sw-upgrade's
// chromium+firefox, and min-version-tests.yml's `browser-minimum` job
// (Firefox 111 / Chrome for Testing 115). WebKit -- the engine the floor's
// binding constraint actually names -- was never run at all.
//
// This is a real-engine SMOKE check, not a pinned-floor check like
// min-version-tests.yml's `browser-minimum`: Firefox/Chrome for Testing both
// publish a downloadable, versioned old release to pin against (see that
// job's own comments); there is no equivalent for WebKit, so this runs
// against whatever *current* WebKit build Playwright ships. It proves the
// production bundle (built against the browserslist target that includes
// Safari 16.4) actually loads and functions end to end on the engine in
// question -- login, session, dashboard render -- which nothing else in CI
// does.
//
// It deliberately does NOT assert the Push API surface is present. Verified
// for this issue (2026-09-14) with a standalone WebKit2GTK 4.1
// GObject-introspection smoke check (same open-source WebKit codebase
// Playwright's own "webkit" channel is built from, minus Apple's private,
// macOS-only push-service frameworks): `'serviceWorker' in navigator` is
// true, but `'PushManager' in window` is false. Real Safari's Web Push
// support is tied to those proprietary frameworks, which no open-source
// WebKit build -- Playwright's or WebKitGTK's -- replicates, and there is no
// real-Safari CI runner available here. So the floor's Web-Push rationale
// itself stays genuinely unverifiable in CI; the test below pins that
// absence explicitly (per this issue's own disposition: "explicitly record
// that the stated floor rationale is unverified") rather than silently
// assuming a capability nothing here can check, or asserting something that
// would fail for a reason unrelated to this project's code. If a future
// WebKit ships PushManager, this assertion breaks -- on purpose, as the
// signal to strengthen it into a real capability check and update the two
// docs above.
//
// Like notifications.spec.ts's push-subscription test, no spec in this repo
// -- on any engine -- performs a live subscribe() round trip against a real
// push service; that is exercised through the API directly.
test.describe('WebKit smoke (issue #992)', () => {
  test('the app loads and functions on WebKit; the Push API surface is absent', async ({
    page,
  }) => {
    await page.goto('/');

    // Logged-out: the login page renders and is usable (mirrors
    // auth.spec.ts's "should display login page when not authenticated").
    await expect(page.getByRole('heading', { name: /login/i })).toBeVisible();
    await expect(page.getByLabel(/username or email/i)).toBeVisible();
    await expect(page.getByLabel(/password/i)).toBeVisible();

    await page.getByLabel(/username or email/i).fill(TEST_USER.username);
    await page.getByLabel(/password/i).fill(TEST_USER.password);
    await page.getByRole('button', { name: /login/i }).click();

    // A real, authenticated route renders -- proves routing, API calls, and
    // MUI rendering all work on this engine, not just the static shell.
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({
      timeout: 10000,
    });

    // The service worker registers on window load (see serviceWorker.spec.ts
    // for why this matters: a PushSubscription is owned by the
    // registration). This is real, working WebKit coverage of that path.
    await expect
      .poll(
        () => page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length),
        { message: 'the app should register its service worker on load', timeout: 15000 },
      )
      .toBeGreaterThan(0);

    // See the file header: this is a known, verified absence in every
    // CI-available WebKit build, not a regression to chase.
    const capabilities = await page.evaluate(() => ({
      serviceWorker: 'serviceWorker' in navigator,
      pushManager: 'PushManager' in window,
    }));
    expect(capabilities.serviceWorker, 'WebKit should expose the serviceWorker API').toBe(true);
    expect(
      capabilities.pushManager,
      'WebKit unexpectedly exposes PushManager -- the Push API is now verifiable on this engine; ' +
        'loosen this assertion to `.toBe(true)` and update the "Browsers" row notes in ' +
        'docs/development/supported-runtime-matrix.md and docs/supported-versions.md',
    ).toBe(false);
  });
});
