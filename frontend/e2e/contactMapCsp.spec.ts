import { expect, test, waitForLoading } from './fixtures';

/**
 * Issue #1286 follow-up: the SPA Content-Security-Policy shipped with
 * `connect-src 'self'` and no `worker-src`, so in the production nginx image the
 * browser blocked MapLibre's blob: worker and every fetch to the tile host. Map
 * markers are DOM overlays, so the page still "worked" — pins on a blank canvas
 * — and no header-string assertion could see it. This drives the real built
 * image in a real browser and asserts the browser itself raises no CSP
 * violation and actually lets the style request through.
 *
 * The tile host is the e2e compose default (OpenFreeMap), stubbed here so the
 * run needs no internet: Chromium enforces connect-src *before* the network, so
 * a stubbed request reaching the route handler proves the CSP allowed it, and a
 * blocked one never does.
 */
const TILE_ORIGIN = 'https://tiles.openfreemap.org';

test.describe('contact map CSP (#1286)', () => {
  test('the map page loads its style and worker with no CSP violations', async ({
    page,
    browserName,
  }) => {
    // The nightly Firefox project runs headless in CI without WebGL, so
    // maplibre throws at construction and ContactMap renders its "map
    // unavailable" alert instead of the `contact-map` region this asserts on.
    // The CSP itself is server-side and engine-independent: Chromium covers
    // the violation probe and the header test below runs on every engine.
    test.skip(
      browserName === 'firefox',
      'headless CI Firefox has no WebGL, so the map never mounts',
    );
    const violations: string[] = [];
    await page.exposeFunction('__recordCspViolation', (v: string) => violations.push(v));
    await page.addInitScript(() => {
      document.addEventListener('securitypolicyviolation', (e) => {
        (window as unknown as Record<string, (v: string) => void>).__recordCspViolation(
          `${e.violatedDirective} blocked ${e.blockedURI}`,
        );
      });
    });

    let styleRequests = 0;
    await page.route(`${TILE_ORIGIN}/**`, (route) => {
      styleRequests += 1;
      return route.fulfill({
        contentType: 'application/json',
        json: { version: 8, sources: {}, layers: [] },
      });
    });
    // Serve the style URL from the allowed origin whatever the instance is
    // configured with, so the assertion is about that origin and no other.
    await page.route('**/api/v1/config/map', (route) =>
      route.fulfill({ json: { tile_style_url: `${TILE_ORIGIN}/styles/liberty` } }),
    );

    await page.goto('/map');
    await waitForLoading(page);
    await expect(page.getByTestId('contact-map')).toBeVisible();

    await expect
      .poll(() => styleRequests, { message: 'style request reached the tile origin' })
      .toBeGreaterThan(0);
    // MapLibre builds its worker from a blob: URL, but an empty stub style may
    // never spawn one, so probe the same operation directly: a CSP without
    // worker-src blob: raises a violation here regardless of the style.
    await page.evaluate(() => {
      const url = URL.createObjectURL(new Blob([''], { type: 'text/javascript' }));
      try {
        new Worker(url).terminate();
      } catch {
        // a blocked worker may also throw synchronously; the event is the signal
      }
      URL.revokeObjectURL(url);
    });
    // Let the worker spin up and any blocked follow-up requests surface.
    await page.waitForTimeout(500);
    expect(violations).toEqual([]);
  });

  test('the served SPA CSP allows the tile origin and a blob: worker, and nothing broader', async ({
    request,
  }) => {
    const res = await request.get('/');
    const csp = res.headers()['content-security-policy'] ?? '';
    expect(csp).toContain(`connect-src 'self' ${TILE_ORIGIN}`);
    expect(csp).toContain(`img-src 'self' data: blob: ${TILE_ORIGIN}`);
    expect(csp).toContain("worker-src 'self' blob:");
    expect(csp).not.toMatch(/\s(https?:|\*)(\s|;)/); // no blanket scheme/wildcard
  });
});
