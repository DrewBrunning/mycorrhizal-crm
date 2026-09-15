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
// It also asserts the Push API surface itself is present -- confirmed on the
// real Playwright-shipped WebKit build in CI for this issue (2026-09-14).
// That corrects an earlier draft of this spec, which asserted the opposite
// based on a standalone WebKit2GTK 4.1 GObject-introspection check (the
// distro WebKitGTK 2.52.6 package, not Playwright's bundled build): that
// engine reported `'serviceWorker' in navigator` true but `'PushManager' in
// window` false. The two turned out not to be equivalent stand-ins -- real
// CI on Playwright's actual webkit channel shows `PushManager` IS present.
// Lesson for next time: a local proxy engine is a reasonable first check but
// not a substitute for confirming against the exact binary CI will run.
//
// This still isn't a full verification of the floor's Web-Push rationale: no
// spec in this repo, on any engine -- see notifications.spec.ts's
// push-subscription test -- performs a live subscribe() round trip against a
// real push service, and there is still no real-Safari CI runner anywhere to
// compare against. What this pins is the capability surface the app's own
// browserSupportsPush() checks for, on the actual engine CI can run.
test.describe('WebKit smoke (issue #992)', () => {
  test('the app loads and functions on WebKit, with the Push API surface present', async ({
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

    // The exact binding constraint docs/development/supported-runtime-matrix.md's
    // "Browsers" row names for the Safari/iOS >=16.4 floor: browserSupportsPush()
    // in frontend/src/pushSubscription.ts checks for these same two globals.
    const capabilities = await page.evaluate(() => ({
      serviceWorker: 'serviceWorker' in navigator,
      pushManager: 'PushManager' in window,
    }));
    expect(capabilities.serviceWorker, 'WebKit should expose the serviceWorker API').toBe(true);
    expect(
      capabilities.pushManager,
      'WebKit no longer exposes PushManager on the Playwright build CI runs -- if this regressed ' +
        'upstream, update this assertion and the "Browsers" row notes in ' +
        'docs/development/supported-runtime-matrix.md and docs/supported-versions.md to say the ' +
        'Push API surface is unverifiable again',
    ).toBe(true);
  });
});
