import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';

// T32: Network, Settings and User Management were found broken at phone widths
// during real-world testing — the same class of fix T28 applied to the contact
// page. These specs pin the invariant the ticket demands: none of the three
// pages overflows the page horizontally at phone widths, and User Management's
// user list stays readable and actionable (stacked cards, not a clipped table).
async function noHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
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
  // The shared `testuser` is the auto-admin; per-worker users are not.
  test.use({ viewport: { width: 360, height: 800 }, sharedUser: true });

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
  test.use({ viewport: { width: 414, height: 896 }, sharedUser: true });

  test('user list does not overflow at 414px either', async ({ page }) => {
    await page.goto('/users');
    await expect(page.getByRole('heading', { name: /user management/i })).toBeVisible();
    await noHorizontalOverflow(page);
  });
});

// Issue #1785: page-header action buttons used to be uppercase MUI Buttons in a
// non-wrapping row, so long German/French labels pushed the page wider than the
// viewport ("AKTUALISIEREN" +50px on /system-status, "DIAGNOSE AUSFÜHREN" +12px
// on /system-events, the triage stepper +8px, /activities +5px). The shared
// PageHeader wraps and shrinks its actions; this sweep pins the outcome in a real
// browser for every header-bearing route in every shipped locale.
const HEADER_ROUTES = [
  '/contacts',
  '/activities',
  '/notes',
  '/occasions',
  '/households',
  '/circles',
  '/shares',
  '/audit',
  '/settings',
  '/settings/data',
  '/users',
  '/system-events',
  '/system-status',
  '/circle-tag-triage',
];
const HEADER_LOCALES = ['de', 'fr', 'es', 'it', 'en'];
const HEADER_WIDTHS = [320, 360];

async function documentOverflowPx(page: Page): Promise<number> {
  return page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
}

test.describe('Mobile layout: page headers wrap in every locale (#1785)', () => {
  // The shared `testuser` is the auto-admin, so the admin-only routes render.
  test.use({ sharedUser: true });

  for (const locale of HEADER_LOCALES) {
    for (const width of HEADER_WIDTHS) {
      for (const route of HEADER_ROUTES) {
        test(`${route} has no horizontal overflow at ${width}px in ${locale}`, async ({ page }) => {
          // i18next's language detector reads localStorage before navigator.
          await page.addInitScript((lng) => {
            localStorage.setItem('i18nextLng', lng);
          }, locale);
          await page.setViewportSize({ width, height: 800 });
          await page.goto(route);
          await expect(page.locator('h1').first(), `${route} renders its heading`).toBeVisible();
          // The sweep is only a locale sweep if the locale actually applied.
          await expect(page.locator('html')).toHaveAttribute('lang', locale);
          // Let the lazily loaded panels (system events, triage) lay out.
          await page.waitForLoadState('networkidle');
          expect(await documentOverflowPx(page), 'page overflows horizontally (px)').toBe(0);
          // Header actions stay reachable: phone touch targets are >= 44px.
          for (const button of await page
            .getByTestId('page-header-actions')
            .locator('button')
            .all()) {
            const box = await button.boundingBox();
            if (box) expect(box.height, 'header action height').toBeGreaterThanOrEqual(44);
          }
          // ...and so are icon buttons beside the title (the Activities/Notes info button).
          for (const iconButton of await page
            .getByTestId('page-header')
            .locator('.MuiIconButton-root')
            .all()) {
            const box = await iconButton.boundingBox();
            if (box) {
              expect(box.height, 'header icon button height').toBeGreaterThanOrEqual(44);
              expect(box.width, 'header icon button width').toBeGreaterThanOrEqual(44);
            }
          }
        });
      }
    }
  }
});
