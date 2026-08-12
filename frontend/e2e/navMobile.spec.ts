import { test, expect } from './fixtures';
import type { Page } from '@playwright/test';

// T33: the global nav bar had accumulated ten destinations and was "incredibly
// crowded" at phone widths. At <sm the AppBar now shows only the primary
// destinations as icon-only buttons (with aria-labels), secondary items live
// in the hamburger drawer, and account-level items collapse into the account
// menu. These specs pin that structure and that nothing overflows at 360-414px.
async function noHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(overflow, 'the nav bar must not push the page wider than the viewport').toBe(false);
}

test.describe('Mobile navigation (T33)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('primary destinations stay directly visible as icon-only buttons', async ({ page }) => {
    await page.goto('/contacts');
    // Icon-only affordances keep their accessible names (aria-label).
    await expect(page.getByRole('link', { name: 'Contacts' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Notes' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Account' })).toBeVisible();
    await noHorizontalOverflow(page);
  });

  test('a primary icon navigates directly', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: 'Notes' }).click();
    await expect(page).toHaveURL(/\/notes$/);
  });

  test('secondary destinations are reachable via the hamburger drawer in one tap', async ({ page }) => {
    await page.goto('/network');
    await page.getByRole('button', { name: 'menu' }).click();
    for (const name of [/dashboard/i, /households/i, /activities/i, /network/i, /shares/i]) {
      await expect(page.getByRole('link', { name })).toBeVisible();
    }
  });

  test('account-level destinations collapse into the account menu', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Account' }).click();
    await expect(page.getByRole('menuitem', { name: /settings/i })).toBeVisible();
    await expect(page.getByRole('menuitem', { name: /logout/i })).toBeVisible();

    await page.getByRole('menuitem', { name: /settings/i }).click();
    await expect(page).toHaveURL(/\/settings$/);
  });
});

test.describe('Mobile navigation at 360px (T33)', () => {
  test.use({ viewport: { width: 360, height: 800 } });

  test('the AppBar does not crowd or overflow at 360px', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('link', { name: 'Contacts' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Account' })).toBeVisible();
    await noHorizontalOverflow(page);
  });
});
