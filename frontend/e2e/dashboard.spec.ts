import { expect, test, waitForLoading } from './fixtures';

// Authenticated via the shared storageState (see playwright.config.ts).
//
// Tagged @prod-defaults (issue #274): part of the small subset re-run under
// production-default settings (real rate limit, CardDAV off) — the dashboard
// is the post-login landing page, and the navigation tests cover the sidebar
// the whole session depends on.
test.describe('Dashboard', { tag: '@prod-defaults' }, () => {
  test('should display dashboard', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
  });

  test('should display three dashboard sections', async ({ page }) => {
    await page.goto('/');
    await waitForLoading(page);

    await expect(page.getByText('Upcoming Birthdays').first()).toBeVisible();
    await expect(page.getByText('Upcoming Reminders').first()).toBeVisible();
    await expect(page.getByText('Stay in Touch').first()).toBeVisible();
  });

  test('should render the Stay in Touch section', async ({ page }) => {
    await page.goto('/');
    await waitForLoading(page);

    const stayInTouchSection = page.getByText('Stay in Touch').locator('..');
    await expect(stayInTouchSection).toBeVisible();
  });
});

test.describe('Navigation', { tag: '@prod-defaults' }, () => {
  test('should navigate to contacts from sidebar', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: /contacts/i }).click();
    await expect(page).toHaveURL(/\/contacts/);
    await expect(page.getByRole('heading', { name: /contacts/i })).toBeVisible();
  });

  test('should navigate to activities from sidebar', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: /activities/i }).click();
    await expect(page).toHaveURL(/\/activities/);
  });

  test('should navigate to settings from sidebar', async ({ page }) => {
    await page.goto('/');
    // The sidebar is a flat list — one item goes straight to /settings,
    // there is no submenu to expand. Its i18n key is `nav.profile` but the
    // rendered label is "Settings" (a stale key name from before a rename).
    await page.locator('.MuiDrawer-root a[href="/settings"]').click();
    await expect(page).toHaveURL(/\/settings/);
  });
});
