import { test, expect } from './fixtures';
import type { Page } from '@playwright/test';

// T32: Network, Settings and User Management were found broken at phone widths
// during real-world testing — the same class of fix T28 applied to the contact
// page. These specs pin the invariant the ticket demands: none of the three
// pages overflows the page horizontally at phone widths, and User Management's
// user list stays readable and actionable (stacked cards, not a clipped table).
async function noHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(overflow, 'the page must not scroll horizontally').toBe(false);
}

test.describe('Mobile layout: Network (T32)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('graph is viewable without page-level horizontal overflow', async ({ page }) => {
    await page.goto('/network');
    await expect(page.getByRole('heading', { name: /contact network/i })).toBeVisible();
    // Seeded contacts exist, so the graph canvas renders.
    await expect(page.locator('canvas').first()).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

test.describe('Mobile layout: Settings (T32)', () => {
  test.use({ viewport: { width: 360, height: 800 } });

  test('sections reflow without page-level horizontal overflow', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.getByRole('heading', { name: /settings/i })).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

test.describe('Mobile layout: User Management (T32)', () => {
  test.use({ viewport: { width: 360, height: 800 } });

  test('user list reflows to stacked cards, readable and actionable', async ({ page }) => {
    await page.goto('/users');
    await expect(page.getByRole('heading', { name: /user management/i })).toBeVisible();

    // The seeded test user is the first registered account (auto-admin), so
    // it appears in the list. At phone width it renders as a card, not a table.
    await expect(page.getByText('testuser', { exact: true })).toBeVisible();
    await expect(page.getByRole('table')).toHaveCount(0);
    // Edit/delete actions stay reachable on the stacked layout.
    await expect(page.getByTitle('Edit').first()).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

test.describe('Mobile layout: User Management at 414px (T32)', () => {
  test.use({ viewport: { width: 414, height: 896 } });

  test('user list does not overflow at 414px either', async ({ page }) => {
    await page.goto('/users');
    await expect(page.getByRole('heading', { name: /user management/i })).toBeVisible();
    await noHorizontalOverflow(page);
  });
});
