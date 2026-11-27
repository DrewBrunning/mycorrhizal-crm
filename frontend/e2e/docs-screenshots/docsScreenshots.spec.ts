import * as path from 'node:path';
import { expect, type Page, test } from '@playwright/test';

/**
 * Generates the screenshots the user documentation embeds
 * (docs/assets/screenshots/). Issue #2244.
 *
 * Needs the demo stack's credentials in the environment:
 *   DOCS_BASE_URL   origin of the stack (default http://localhost:7300)
 *   DOCS_USERNAME   the seeded user
 *   DOCS_PASSWORD   its password (docker exec ... cat /app/data/credentials.json)
 * scripts/docs-screenshots.sh reads them from the running stack.
 */
const OUT = path.resolve(import.meta.dirname, '../../../docs/assets/screenshots');

// A contact in the demo manifest (backend/cmd/pentestseed, `demo` profile).
const FEATURED_CONTACT = 'Marcus Bell';

const username = process.env.DOCS_USERNAME;
const password = process.env.DOCS_PASSWORD;

async function login(page: Page) {
  await page.goto('/');
  await page.getByLabel(/username or email/i).fill(username as string);
  await page.getByLabel(/password/i).fill(password as string);
  await page.getByRole('button', { name: /login/i }).click();
  await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({ timeout: 20000 });
}

async function shot(page: Page, name: string) {
  // Let fonts, images and the post-navigation fetches settle so the PNG is stable.
  // Background polling can keep the network from ever idling, so cap the wait.
  await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {});
  await page.screenshot({ path: path.join(OUT, `${name}.png`) });
}

test.describe('documentation screenshots', () => {
  test.skip(
    !username || !password,
    'set DOCS_USERNAME and DOCS_PASSWORD (see scripts/docs-screenshots.sh)',
  );

  test('capture', async ({ page }) => {
    test.setTimeout(180_000);
    await login(page);

    await page.goto('/contacts');
    await expect(page.getByRole('heading', { name: /contacts/i }).first()).toBeVisible();
    await shot(page, 'contacts-list');

    // The demo profile's first contact in the list (clean, deterministic names).
    await page.getByText(FEATURED_CONTACT).first().click();
    await expect(page).toHaveURL(/\/contacts\/[^/]+$/);
    await expect(page.getByRole('heading', { name: FEATURED_CONTACT }).first()).toBeVisible();
    await shot(page, 'contact-detail');

    // The import dialog, opened from the Contacts page.
    await page.goto('/contacts');
    await page.getByRole('button', { name: /^import$/i }).click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await shot(page, 'import-dialog');

    for (const [route, name] of [
      ['/network', 'network'],
      ['/activities', 'activities'],
      ['/notes', 'notes'],
      ['/occasions', 'occasions'],
    ] as const) {
      await page.goto(route);
      await shot(page, name);
    }
  });
});
