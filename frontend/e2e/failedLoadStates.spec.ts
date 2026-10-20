import type { Page, Route } from '@playwright/test';
import { expect, test } from './fixtures';

// Issue #1779: when a page's data fails to load, the page must say so (shared
// LoadError: role=alert + Retry) instead of presenting an empty state ("No
// notes", "No circles yet", ...) that tells the user their data is gone.
//
// Every /api/v1 data request is answered with a 500 in the backend's real
// error envelope; auth, session, config and health calls pass through so the
// app shell still boots. Each listed route must then show a visible error
// alert and none of its empty-state copy. Route-stubbed (no seeded data), so it
// is fast and deterministic per PR.
test.describe('Failed load states', { tag: '@failure-modes' }, () => {
  // Calls the shell needs to render at all, or that are not page data.
  const PASS_THROUGH =
    /^\/api\/v1\/(auth|health|config|users\/me|preferences|sessions|webauthn\/(?!credentials))/;

  async function failAllDataApis(page: Page) {
    await page.route(
      (url) => url.pathname.startsWith('/api/v1/') && !PASS_THROUGH.test(url.pathname),
      (route: Route) =>
        route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({
            error: { code: 'INTERNAL_ERROR', message: 'Simulated outage' },
            request_id: 'req-e2e-1779',
            timestamp: '2026-10-20T00:00:00Z',
          }),
        }),
    );
  }

  // Empty-state copy (en) that would be a lie on a failed load.
  const EMPTY_COPY =
    /No unfiled notes|No activities found|No events yet|No passkeys yet|No webhooks|No custom fields|No calendar|No households|No circles yet|No tags yet|No incoming|No outgoing|No audit|No events|No users|No legacy circles/i;

  const routes: Array<{ path: string; name: string }> = [
    { path: '/notes', name: 'Notes' },
    { path: '/activities', name: 'Activities' },
    { path: '/occasions', name: 'Occasions' },
    { path: '/settings', name: 'Settings' },
    { path: '/settings/data', name: 'Data settings' },
    { path: '/households', name: 'Households' },
    { path: '/circles', name: 'Circles' },
    { path: '/circles?tab=tags', name: 'Tags' },
    { path: '/shares', name: 'Shares' },
    { path: '/audit', name: 'Audit log' },
    { path: '/users', name: 'Users' },
    { path: '/system-events', name: 'System events' },
    { path: '/circle-tag-triage', name: 'Circle and tag triage' },
    // Already correct before #1779; pinned so they stay that way.
    { path: '/', name: 'Dashboard' },
    { path: '/contacts', name: 'Contacts' },
    { path: '/network', name: 'Network' },
  ];

  for (const { path, name } of routes) {
    test(`${name} (${path}): a 500 shows an error alert, not an empty state`, async ({ page }) => {
      await failAllDataApis(page);
      await page.goto(path);

      await expect(page.getByRole('alert').first()).toBeVisible();
      await expect(page.locator('main, body').first()).not.toContainText(EMPTY_COPY);
    });
  }

  test('Retry refetches in place once the API recovers (no reload)', async ({ page }) => {
    await failAllDataApis(page);
    await page.goto('/notes');
    const alert = page.getByRole('alert').filter({ hasText: /couldn.t load this data/i });
    await expect(alert).toBeVisible();
    await expect(alert).toContainText('req-e2e-1779');

    await page.unrouteAll({ behavior: 'ignoreErrors' });
    await alert.getByRole('button', { name: 'Retry' }).click();
    await expect(alert).toBeHidden();
    // The recovered, genuinely-empty-or-populated list: the failure copy is gone.
    await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible();
  });
});
