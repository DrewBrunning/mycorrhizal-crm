import { type APIRequestContext, request as apiRequest, type Page } from '@playwright/test';
import {
  assertNoBlockingA11yViolations,
  assertNoRepeatedAccessibleNames,
  createTestContact,
  deleteThrowawayUser,
  expect,
  makeThrowawayUser,
  test,
  uniqueToken,
} from './fixtures';
import { API_BASE_URL, APP_ORIGIN } from './global-setup';

/**
 * Targeted populated-state coverage for the accessible-name gate (issue #2920
 * item 5). The shared fixture blocks on `heading-order` and
 * `label-content-name-mismatch` at any impact, and exports the repeated-name
 * check for states where it is deterministic. These are the populated states
 * the audit found:
 *
 * - #2906: a phone-width header with a pending share. The hamburger's
 *   aria-label overrides the Badge's name, so the visible "1" is missing from
 *   the accessible name (label-content-name-mismatch).
 * - #2910: the import dialog jumps h2 -> h6 on the upload and mapping steps
 *   (heading-order).
 * - #2911: the duplicate-suggestion list repeats "Merge…" and "Not a
 *   duplicate" once per pair, and the add-contact dialog has four "Add"
 *   buttons (the custom repeated-name check).
 *
 * These specs assert the shared checks, not the fixes: they are red on main
 * until the corresponding fix PRs land, which is the point of the ratchet.
 */

/**
 * Registers a throwaway sender, has it share a contact with the calling (worker)
 * user, and returns the sender username for cleanup. The grant stays pending, so
 * the app renders the nav badge. Deleting the sender cascades its grant.
 */
async function seedPendingShare(page: Page): Promise<string> {
  const me = await (await page.request.get(`${API_BASE_URL}/users/me`)).json();
  const sender = makeThrowawayUser('a11y_names_sender');
  const api: APIRequestContext = await apiRequest.newContext({ baseURL: APP_ORIGIN });
  try {
    const registered = await api.post(`${API_BASE_URL}/register`, { data: sender });
    expect(registered.ok(), `register sender: ${await registered.text()}`).toBeTruthy();
    const login = await api.post(`${API_BASE_URL}/login`, {
      data: { identifier: sender.username, password: sender.password },
    });
    expect(login.ok(), `login sender: ${await login.text()}`).toBeTruthy();

    const contact = await createTestContact(api, {
      firstname: `A11yShare${uniqueToken()}`,
      lastname: 'Pending',
    });
    const grant = await api.post(`${API_BASE_URL}/share-grants`, {
      headers: { 'Idempotency-Key': `a11y-pending-${uniqueToken()}` },
      data: {
        to_user_id: me.id,
        entity_type: 'contact',
        entity_id: contact.uid,
        sections: ['emails'],
      },
    });
    expect(grant.ok(), `create pending share grant: ${await grant.text()}`).toBeTruthy();
  } finally {
    await api.dispose();
  }
  return sender.username;
}

test.describe('accessible names in populated states', () => {
  test('phone header with a pending share keeps its visible name (#2906)', async ({ page }) => {
    const sender = await seedPendingShare(page);
    try {
      await page.setViewportSize({ width: 375, height: 800 });
      await page.goto('/');
      // Both the pre-fix Badge (role=img, name "1 share waiting…") and the
      // fixed aria-hidden badge leave this visible count in the DOM. Scope to
      // the hamburger; the nav's Shared-with-me and Settings entries carry the
      // same count badge at this width.
      await expect(
        page.getByRole('button', { name: /open navigation/i }).locator('.MuiBadge-badge'),
      ).toHaveText('1');
      await assertNoBlockingA11yViolations(page);
    } finally {
      await deleteThrowawayUser(sender);
    }
  });

  test('import dialog steps keep heading order (#2910)', async ({ page }) => {
    await page.goto('/contacts');
    await page.getByRole('button', { name: /import/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await assertNoBlockingA11yViolations(page, '[role="dialog"]');

    // The same h2 -> h6 skip exists on the mapping step
    // (ImportFixedAssignmentPicker), so exercise it too.
    await dialog.locator('#import-file-input').setInputFiles({
      name: 'a11y-names.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from('First Name,Last Name,Email\nAda,Lovelace,ada@example.com\n'),
    });
    await expect(dialog.getByRole('combobox', { name: 'Map column Email to' })).toBeVisible();
    await assertNoBlockingA11yViolations(page, '[role="dialog"]');
  });

  test('duplicate suggestions name each pair (#2911)', async ({ page, request }) => {
    const runId = uniqueToken();
    // Word breaks are ignored by the normalized tier, so each pair is a real
    // suggestion while the two sides still read distinctly in its label.
    const names = [
      `A11yAlpha${runId}`,
      `A11y Alpha ${runId}`,
      `A11yBeta${runId}`,
      `A11y Beta ${runId}`,
    ] as const;
    const created: string[] = [];

    try {
      for (const name of names) {
        const res = await request.post(`${API_BASE_URL}/entities`, {
          data: { kind: 'organisation', name, core: { organization: { name } } },
        });
        expect(res.ok(), `create entity ${name}: ${await res.text()}`).toBeTruthy();
        created.push((await res.json()).id);
      }

      await expect
        .poll(
          async () => {
            const res = await request.get(`${API_BASE_URL}/entities/suggestions?kind=organisation`);
            if (!res.ok()) return 0;
            return ((await res.json()).pairs ?? []).length;
          },
          { message: 'two entity pairs must be offered before scanning', timeout: 10000 },
        )
        .toBeGreaterThanOrEqual(2);

      await page.goto('/entities');
      await page.getByRole('button', { name: 'Find duplicates' }).click();
      const panel = page.locator('section', {
        has: page.getByRole('heading', { name: 'Possible duplicates' }),
      });
      await expect(panel.getByRole('listitem')).toHaveCount(2, { timeout: 10000 });
      // The two pair rows each render "Merge…" and "Not a duplicate"; the
      // shared repeated-name check is what must distinguish them.
      await assertNoRepeatedAccessibleNames(page, 'main');
    } finally {
      for (const id of created) {
        await request.delete(`${API_BASE_URL}/entities/${id}`).catch(() => {});
      }
    }
  });

  test('add-contact field actions are distinguishable (#2911)', async ({ page }) => {
    await page.goto('/contacts');
    await page.getByRole('button', { name: /add/i }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    // The phone and email MultiValueFields plus the tag editor each render a
    // bare "Add" on main's pre-fix tree. Check the repeated-name rule directly:
    // this dialog also has a pre-existing heading-order offender that is not
    // part of #2911, and the import test above is the heading-order coverage.
    await expect(dialog.getByRole('button', { name: 'Add' }).first()).toBeVisible();
    await assertNoRepeatedAccessibleNames(page, '[role="dialog"]');
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
  });
});
