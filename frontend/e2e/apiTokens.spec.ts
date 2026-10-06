import { expect, LOGGED_OUT, test, waitForLoading } from './fixtures';
import { API_BASE_URL, APP_ORIGIN } from './global-setup';

// Issue #1481: API tokens had only incidental e2e coverage. This drives the
// whole lifecycle through the real stack: create in Settings -> the one-time
// secret is shown (and copyable) -> it authenticates a cookie-less client
// against /api/v1/contacts -> it is never shown again after reload -> revoking
// it makes the same bearer 401.

test.describe('API tokens', () => {
  test('create, use, hide-after-reload, revoke', async ({ page, playwright }) => {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    const name = `e2e-token-${Date.now()}`;

    await page.goto('/settings');
    await waitForLoading(page);

    // --- create ---
    await page.getByRole('button', { name: 'Create Token' }).click();
    const createDialog = page.getByRole('dialog', { name: 'Create API Token' });
    await createDialog.getByLabel('Token Name').fill(name);
    await createDialog.getByRole('button', { name: 'Create', exact: true }).click();

    // --- the one-time secret ---
    const created = page.getByRole('dialog', { name: 'Token Created' });
    await expect(created).toBeVisible();
    await expect(
      created.getByText('This token will not be shown again.', { exact: false }),
    ).toBeVisible();
    const secretField = created.getByRole('textbox');
    const secret = await secretField.inputValue();
    expect(secret.length).toBeGreaterThan(20);

    // The copy button puts exactly that secret on the clipboard.
    await created.getByRole('button', { name: 'Copy' }).click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(secret);
    await created.getByRole('button', { name: 'Done, I saved it' }).click();
    await expect(created).toBeHidden();

    const row = page.getByRole('row', { name: new RegExp(name) });
    await expect(row.getByText('Active')).toBeVisible();

    // --- the secret authenticates a client with no session cookie ---
    const bearer = await playwright.request.newContext({
      baseURL: APP_ORIGIN,
      storageState: LOGGED_OUT,
      extraHTTPHeaders: { Authorization: `Bearer ${secret}` },
    });
    try {
      const ok = await bearer.get(`${API_BASE_URL}/contacts?limit=1`);
      expect(ok.status()).toBe(200);
      const anon = await playwright.request.newContext({
        baseURL: APP_ORIGIN,
        storageState: LOGGED_OUT,
      });
      try {
        expect((await anon.get(`${API_BASE_URL}/contacts?limit=1`)).status()).toBe(401);
      } finally {
        await anon.dispose();
      }

      // --- never shown again: reload lists the token by name only ---
      await page.reload();
      await waitForLoading(page);
      await expect(page.getByRole('row', { name: new RegExp(name) })).toBeVisible();
      await expect(page.getByText(secret)).toHaveCount(0);
      expect(
        await page
          .locator('input')
          .evaluateAll((els, s) => els.some((e) => (e as HTMLInputElement).value === s), secret),
      ).toBe(false);
      expect(await page.content()).not.toContain(secret);

      // --- revoke -> the same bearer is now rejected ---
      await page
        .getByRole('row', { name: new RegExp(name) })
        .getByRole('button', { name: 'Revoke Token' })
        .click();
      const revoke = page.getByRole('dialog', { name: 'Revoke Token' });
      await revoke.getByRole('button', { name: 'Revoke', exact: true }).click();
      await expect(
        page.getByRole('row', { name: new RegExp(name) }).getByText('Revoked'),
      ).toBeVisible();

      expect((await bearer.get(`${API_BASE_URL}/contacts?limit=1`)).status()).toBe(401);
    } finally {
      await bearer.dispose();
    }
  });
});
