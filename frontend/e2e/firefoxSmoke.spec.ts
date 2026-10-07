import * as fs from 'node:fs';
import {
  createTestContact,
  deleteTestContact,
  expect,
  LOGGED_OUT,
  test,
  waitForLoading,
} from './fixtures';
import { API_BASE_URL } from './global-setup';

// Issue #1479: the first Firefox coverage of the app's own UI. Before this the
// only non-Chromium E2E was webkitSmoke.spec.ts, yet Firefox is the engine most
// Linux self-hosters use and CLAUDE.md already records a Firefox-only failure
// (registering a service worker against the SPA-fallback index.html throws
// "The operation is insecure"). This smoke runs on every PR under the
// `firefox` project; the whole chromium suite re-runs on Firefox nightly via
// E2E_ALL_BROWSERS=1 (playwright.config.ts).
//
// The flows are the ones where engines are most likely to disagree: the login
// cookie (SameSite=Strict + HttpOnly handling), contact create/edit (MUI
// forms), date rendering, service-worker registration on the PRODUCTION build
// served by the docker image (frontend-dev never compiles the worker), a CSV
// download (blob/anchor download handling) and logout.
//
// Each test authenticates as the per-worker user (issue #1480) except the
// login/logout one, which signs in through the UI itself.
test.describe('Firefox smoke (issue #1479)', () => {
  test('login sets a HttpOnly SameSite=Strict cookie and logout clears it', async ({
    browser,
    workerUser,
  }) => {
    const context = await browser.newContext({ storageState: LOGGED_OUT });
    try {
      const page = await context.newPage();
      await page.goto('/');
      await expect(page.getByRole('heading', { name: /login/i })).toBeVisible();

      await page.getByLabel(/username or email/i).fill(workerUser.username);
      await page.getByLabel(/password/i).fill(workerUser.password);
      await page.getByRole('button', { name: /login/i }).click();
      await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({
        timeout: 10000,
      });

      // credentialStorage.spec.ts asserts these flags on Chromium only.
      const authCookie = (await context.cookies()).find((c) => c.name === 'auth_token');
      expect(authCookie, 'auth_token cookie must be set after login').toBeTruthy();
      expect(authCookie?.httpOnly).toBe(true);
      expect(authCookie?.sameSite).toBe('Strict');
      // Not readable from script: HttpOnly really is enforced by this engine.
      expect(await page.evaluate(() => document.cookie)).not.toContain('auth_token');

      await page.getByRole('button', { name: /logout/i }).click();
      await expect(page.getByRole('heading', { name: /login/i })).toBeVisible({ timeout: 10000 });
      // The session is really gone, not just the screen.
      const me = await context.request.get(`${API_BASE_URL}/users/me`);
      expect(me.status()).toBe(401);
    } finally {
      await context.close();
    }
  });

  test('creates a contact in the UI and edits its name', async ({ page }) => {
    const firstname = `FxSmoke${Date.now()}`;

    await page.goto('/contacts');
    await page.getByRole('button', { name: /add/i }).click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await page.getByLabel(/first.*name/i).fill(firstname);
    await page.getByLabel(/last.*name/i).fill('Created');
    await page.getByRole('button', { name: /create/i }).click();

    await expect(page).toHaveURL(/\/contacts\/\d+/);
    await expect(page.getByRole('heading', { name: `${firstname} Created` })).toBeVisible();
    const contactId = page.url().match(/\/contacts\/(\d+)/)?.[1];

    try {
      await page.locator('.edit-icon').first().click();
      await page.getByLabel('Last Name', { exact: true }).fill('Edited');
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByRole('heading', { name: `${firstname} Edited` })).toBeVisible();
    } finally {
      if (contactId) await deleteTestContact(page.request, contactId);
    }
  });

  test('renders a birthday in the date format chosen in Settings', async ({ page, request }) => {
    const contact = await createTestContact(page.request, {
      firstname: 'FxSmokeDate',
      lastname: `D${Date.now()}`,
      birthday: '1990-03-15',
    });

    try {
      await page.goto('/settings');
      await page.getByLabel('Date Format').click();
      await page.getByRole('option', { name: 'US full (MMMM D, YYYY)' }).click();
      await expect
        .poll(
          async () => (await (await request.get(`${API_BASE_URL}/users/me`)).json()).date_format,
        )
        .not.toBe('eu');

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      await expect(page.getByText(/March 15, 1990 \(\d+ years old\)/)).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
      // The worker user is per-worker, but later tests on it expect the default.
      await request
        .patch(`${API_BASE_URL}/users/date-format`, { data: { date_format: 'eu' } })
        .catch(() => {});
    }
  });

  test('registers the service worker on the production build and keeps it across a reload', async ({
    page,
  }) => {
    await page.goto('/');

    // The Firefox-only failure CLAUDE.md records: a registration against the
    // SPA-fallback index.html throws "The operation is insecure". So the script
    // itself must be served as JavaScript, and registration must succeed.
    const script = await page.request.get('/service-worker.js');
    expect(script.status()).toBe(200);
    expect(script.headers()['content-type']).toMatch(/javascript/);

    const registrationCount = () =>
      page.evaluate(async () => (await navigator.serviceWorker.getRegistrations()).length);
    await expect
      .poll(registrationCount, {
        message: 'the app should register its service worker on load',
        timeout: 15000,
      })
      .toBeGreaterThan(0);

    await page.reload();
    await page.waitForTimeout(1500);
    expect(await registrationCount(), 'the registration must survive a reload').toBeGreaterThan(0);
  });

  test('downloads the CSV export through the Settings button', async ({ page }) => {
    const contact = await createTestContact(page.request, {
      firstname: 'FxSmokeExport',
      lastname: `E${Date.now()}`,
    });

    try {
      await page.goto('/settings/data');
      const [download] = await Promise.all([
        page.waitForEvent('download'),
        page.getByRole('button', { name: 'Download CSV' }).click(),
      ]);
      expect(download.suggestedFilename()).toMatch(/^mycorrhizal-export.*\.csv$/);

      const downloadedPath = await download.path();
      const csv = fs.readFileSync(downloadedPath, 'utf8');
      expect(csv).toContain('=== CONTACTS ===');
      expect(csv).toContain(contact.firstname);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
