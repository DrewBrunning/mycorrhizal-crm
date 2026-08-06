import { test, expect } from './fixtures';
import { request } from '@playwright/test';
import { API_BASE_URL } from './global-setup';
import { stableClick, waitForLoading } from './fixtures';

// T39: Add new users from User Management. The shared storageState user
// ("testuser") is the first registered account and therefore auto-admin —
// see mobileLayout.spec.ts's note on the same fact.
test.describe('User Management: Add User (T39)', () => {
  test('an admin creates a new user, who can then log in with the set password', async ({ page }) => {
    const suffix = Date.now();
    const newUser = {
      username: `e2e_created_${suffix}`,
      email: `e2e_created_${suffix}@example.com`,
      password: 'BrandNewPassw0rd!',
    };

    await page.goto('/users');
    await waitForLoading(page);
    await expect(page.getByRole('heading', { name: /user management/i })).toBeVisible();

    await stableClick(page.getByRole('button', { name: /add user/i }));
    await expect(page.getByRole('dialog')).toBeVisible();

    await page.getByLabel(/username/i).fill(newUser.username);
    await page.getByLabel(/email/i).fill(newUser.email);
    await page.getByLabel(/password/i).fill(newUser.password);
    await page.getByRole('button', { name: /^save$/i }).click();

    // Dialog closes and the new user shows up in the list.
    await expect(page.getByRole('dialog')).toBeHidden();
    await expect(page.getByText(newUser.username, { exact: true })).toBeVisible();

    // Look up the created user's id as the admin BEFORE logging in as them
    // below — page.request shares the browser context's cookie jar with the
    // page, so a login response's Set-Cookie would otherwise silently swap
    // out the admin session out from under later page.request calls.
    const list = await page.request.get(`${API_BASE_URL}/admin/users?limit=200`);
    expect(list.ok()).toBeTruthy();
    const listBody = await list.json();
    const created = (listBody.users || []).find((u: any) => u.username === newUser.username);
    expect(created, 'created user should be findable via the admin list endpoint').toBeTruthy();

    // Hand-verification from the ticket's "Done when", pinned as an
    // automated check: the created user can actually log in. Uses a wholly
    // separate request context (own cookie jar) so this doesn't clobber the
    // admin session used for cleanup below — mirrors isolation.spec.ts's
    // pattern for acting as a second user.
    const newUserCtx = await request.newContext();
    try {
      const loginResponse = await newUserCtx.post(`${API_BASE_URL}/login`, {
        data: { identifier: newUser.username, password: newUser.password },
      });
      expect(loginResponse.ok(), `login as newly created user failed: ${await loginResponse.text()}`).toBeTruthy();
    } finally {
      await newUserCtx.dispose();
      // Clean up via the admin API, still on the original (admin) session.
      await page.request.delete(`${API_BASE_URL}/admin/users/${created.id}`).catch(() => {});
    }
  });

  test('creating a user with a duplicate username surfaces the conflict and keeps the dialog open', async ({ page }) => {
    await page.goto('/users');
    await waitForLoading(page);
    await expect(page.getByRole('heading', { name: /user management/i })).toBeVisible();

    await stableClick(page.getByRole('button', { name: /add user/i }));
    await expect(page.getByRole('dialog')).toBeVisible();

    // "testuser" is the shared storageState account and already exists.
    await page.getByLabel(/username/i).fill('testuser');
    await page.getByLabel(/email/i).fill(`e2e_dup_${Date.now()}@example.com`);
    await page.getByLabel(/password/i).fill('BrandNewPassw0rd!');
    await page.getByRole('button', { name: /^save$/i }).click();

    // The dialog stays open with a visible error instead of silently closing.
    await expect(page.getByRole('dialog')).toBeVisible();
    await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();
  });
});
