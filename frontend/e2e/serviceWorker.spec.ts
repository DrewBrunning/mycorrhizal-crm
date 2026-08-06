import { test, expect } from './fixtures';

// The service worker must stay registered across page loads.
//
// This is a regression guard for a bug that shipped in v0.2.0 and was found by
// review before v0.3.0: index.tsx still called CRA's scaffolded
// serviceWorkerRegistration.unregister(), which runs on every page load and
// tears down whatever registration it finds. Since a PushSubscription is owned
// by the service worker registration, N9's browser-push channel could never
// survive a reload -- a user enabled it, the subscription was destroyed on the
// next load, and the server's stored row went stale and was pruned on the
// following 404/410. Silently.
//
// Nothing in the unit tests or the rest of the e2e suite could see this:
// notifications.spec.ts exercises push-subscription CRUD through the API,
// which never involves a browser registration at all. It only shows up across
// a real page load, which is exactly what this spec does.
//
// Note this runs against the production build served by the all-in-one image
// (nginx on 7300) -- register() is deliberately a no-op in development builds,
// so a dev-server run would prove nothing.
test.describe('Service worker', () => {
  test('registers and survives a page reload, so a push subscription can outlive it', async ({ page }) => {
    await page.goto('/');

    // Registration happens on window's load event, so poll rather than assume
    // it has already happened by the time the navigation promise resolves.
    await expect
      .poll(
        () => page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length),
        { message: 'the app should register its service worker on load', timeout: 15000 }
      )
      .toBeGreaterThan(0);

    const scope = await page.evaluate(async () => {
      const registrations = await navigator.serviceWorker.getRegistrations();
      return registrations[0].scope;
    });

    await page.reload();

    // The bug: after this reload the count was 0. Give the old unregister()
    // path time to have run before asserting, so a pass cannot be a race.
    await page.waitForTimeout(1500);

    const after = await page.evaluate(async () => {
      const registrations = await navigator.serviceWorker.getRegistrations();
      return { count: registrations.length, scopes: registrations.map((r) => r.scope) };
    });

    expect(after.count, 'the registration must survive a reload').toBeGreaterThan(0);
    expect(after.scopes).toContain(scope);
  });

  test('serves the service worker script with a revalidating cache header', async ({ page }) => {
    // /service-worker.js would otherwise be caught by nginx's 1-year immutable
    // rule for *.js, which is how a deployment gets permanently stuck on an old
    // worker: the file that decides whether any future update is seen would
    // itself be the one cached hardest.
    const response = await page.request.get('/service-worker.js');

    expect(response.status()).toBe(200);
    expect(response.headers()['cache-control']).toContain('no-cache');
  });
});
