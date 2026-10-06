import {
  deleteThrowawayUser,
  expect,
  LOGGED_OUT,
  makeThrowawayUser,
  test,
  waitForLoading,
} from './fixtures';
import { API_BASE_URL } from './global-setup';

// Issue #1481: passkeys (WebAuthn, issues #593/#594) had no Playwright
// coverage; the real ceremony is only exercised server-side
// (controllers/webauthn_virtual_authenticator_test.go) and the settings card
// only against mocks. This drives the whole lifecycle through the shipped
// bundle with Chromium's CDP *virtual authenticator*: register a passkey in
// Settings -> sign out -> the password step now asks for a second factor and
// the passkey completes it -> remove it (proved with a recovery code) -> gone.
//
// Chromium only: the virtual authenticator is a CDP domain, and Firefox/WebKit
// have no equivalent in Playwright (issue #1479 tracks the Firefox project).
test.use({ storageState: LOGGED_OUT });

test.describe('Passkeys', () => {
  test.skip(
    ({ browserName }) => browserName !== 'chromium',
    'needs the Chromium CDP WebAuthn virtual authenticator (no Firefox/WebKit equivalent)',
  );

  test('register, sign in with, and remove a passkey', async ({ page, context }) => {
    const user = makeThrowawayUser('passkey');
    const reg = await page.request.post(`${API_BASE_URL}/register`, { data: user });
    expect(reg.ok(), `registration should succeed: ${await reg.text()}`).toBeTruthy();

    // A software authenticator that auto-answers every ceremony.
    const cdp = await context.newCDPSession(page);
    await cdp.send('WebAuthn.enable');
    const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
      options: {
        protocol: 'ctap2',
        transport: 'internal',
        hasResidentKey: true,
        hasUserVerification: true,
        isUserVerified: true,
        automaticPresenceSimulation: true,
      },
    });

    const login = async () => {
      await page.goto('/');
      await page.getByLabel(/username or email/i).fill(user.username);
      await page.getByLabel(/password/i).fill(user.password);
      await page.getByRole('button', { name: /login/i }).click();
    };

    try {
      await login();
      await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({
        timeout: 15000,
      });

      // --- register ---
      await page.goto('/settings');
      await waitForLoading(page);
      const card = page.locator('.MuiCard-root').filter({
        has: page.getByRole('heading', { name: 'Passkeys' }),
      });
      await expect(card.getByText('No passkeys yet.')).toBeVisible();
      await card.getByLabel('Passkey name (optional)').fill('E2E Laptop');
      await card.getByRole('button', { name: 'Add a passkey' }).click();

      // The first second factor mints one-time recovery codes.
      const recovery = page.getByRole('dialog').filter({ hasText: 'Recovery codes' });
      await expect(recovery).toBeVisible({ timeout: 15000 });
      const codes = await recovery
        .getByText(/^[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}$/)
        .allTextContents();
      expect(codes.length).toBeGreaterThan(0);
      await recovery.getByRole('button', { name: /done/i }).click();

      const list = card.getByRole('list', { name: 'Your passkeys' });
      await expect(list.getByText('E2E Laptop')).toBeVisible();
      await expect(list.getByText(/never used/)).toBeVisible();
      expect(
        (await cdp.send('WebAuthn.getCredentials', { authenticatorId })).credentials,
      ).toHaveLength(1);

      // --- sign out, then the passkey is the second factor ---
      await page.getByRole('button', { name: /logout/i }).click();
      await expect(page.getByRole('heading', { name: /login/i })).toBeVisible({ timeout: 10000 });

      await login();
      await expect(page.getByRole('heading', { name: 'Two-factor authentication' })).toBeVisible();
      await expect(page.getByRole('heading', { name: /dashboard/i })).not.toBeVisible();
      await page.getByRole('button', { name: 'Use a passkey instead' }).click();
      await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({
        timeout: 15000,
      });

      // --- failure: removal with a wrong code is refused, the passkey stays ---
      await page.goto('/settings');
      await waitForLoading(page);
      await card.getByRole('button', { name: 'Remove passkey E2E Laptop' }).click();
      const removeDialog = page.getByRole('dialog', { name: 'Remove passkey' });
      await removeDialog.getByLabel('Verification code *').fill('AAAAA-AAAAA-AAAAA');
      await removeDialog.getByRole('button', { name: 'Remove passkey' }).click();
      await expect(removeDialog.getByRole('alert')).toBeVisible();
      await expect(removeDialog).toBeVisible();

      // --- remove with a real recovery code ---
      await removeDialog.getByLabel('Verification code *').fill(codes[0]);
      await removeDialog.getByRole('button', { name: 'Remove passkey' }).click();
      await expect(removeDialog).toBeHidden();
      await expect(card.getByText('No passkeys yet.')).toBeVisible();
      await page.reload();
      await waitForLoading(page);
      await expect(card.getByText('No passkeys yet.')).toBeVisible();
    } finally {
      await cdp.send('WebAuthn.removeVirtualAuthenticator', { authenticatorId }).catch(() => {});
      // Leave the page before deleting the account: the automatic post-test a11y
      // scan would otherwise run against a still-open app whose session was just
      // deleted out from under it ("Your session has expired" banner).
      await page.goto('about:blank');
      await deleteThrowawayUser(user.username);
    }
  });
});
