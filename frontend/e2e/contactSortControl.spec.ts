import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { E2E_CONTACT_PREFIX } from './global-setup';

/**
 * T77 (docs/fork-plan/tickets/121-T77-web-contacts-list-sort-control.md) — the
 * web sort control that consumes T73's backend ?sort=. The backend ordering
 * itself is pinned by contactSort.spec.ts (API-level); this spec drives the
 * real Contacts page Select and proves the control is wired end to end:
 * default name sort, the chosen sort/order written into the URL (so it
 * survives reload and back-navigation), and a search-isolated list actually
 * reordering when the selection changes.
 */
test.describe('Contacts list sort control (T77)', () => {
  test('defaults to name (A-Z) and persists the chosen sort in the URL', async ({ page }) => {
    await page.goto('/contacts');
    await waitForLoading(page);

    // The control is present and defaults to alphabetical (the frontend's own
    // default — the server keeps updated_at for other API consumers).
    await expect(page.getByLabel('Sort')).toBeVisible();
    await expect(page.getByText('Name (A–Z)')).toBeVisible();

    await page.getByLabel('Sort').click();
    await page.getByRole('option', { name: 'Recently edited (newest first)' }).click();
    await expect(page).toHaveURL(/sort=updated_at/);
    await expect(page).toHaveURL(/order=desc/);

    await page.getByLabel('Sort').click();
    await page.getByRole('option', { name: 'Name (Z–A)' }).click();
    await expect(page).toHaveURL(/sort=name/);
    await expect(page).toHaveURL(/order=desc/);
  });

  test('a sort choice in the URL is honoured on load', async ({ page }) => {
    await page.goto('/contacts?sort=updated_at&order=asc');
    await waitForLoading(page);

    await expect(page.getByText('Recently edited (oldest first)')).toBeVisible();
  });

  test('re-orders a searched list when the sort changes', async ({ page, request }) => {
    const ts = Date.now();
    const created = await Promise.all([
      createTestContact(request, { firstname: `${E2E_CONTACT_PREFIX}T77SortA${ts}`, lastname: 'T77Alpha' }),
      createTestContact(request, { firstname: `${E2E_CONTACT_PREFIX}T77SortZ${ts}`, lastname: 'T77Zulu' }),
      createTestContact(request, { firstname: `${E2E_CONTACT_PREFIX}T77SortM${ts}`, lastname: 'T77Mike' }),
    ]);

    try {
      const search = `${E2E_CONTACT_PREFIX}T77Sort`;
      await page.goto(`/contacts?search=${encodeURIComponent(search)}`);
      await waitForLoading(page);

      const alpha = page.getByText('T77Alpha').first();
      const zulu = page.getByText('T77Zulu').first();
      await expect(alpha).toBeVisible();
      await expect(zulu).toBeVisible();

      // Default name ascending: T77Alpha renders above T77Zulu.
      expect((await alpha.boundingBox())!.y).toBeLessThan((await zulu.boundingBox())!.y);

      // Flip to descending: T77Zulu renders above T77Alpha.
      await page.getByLabel('Sort').click();
      await page.getByRole('option', { name: 'Name (Z–A)' }).click();
      await waitForLoading(page);

      expect((await zulu.boundingBox())!.y).toBeLessThan((await alpha.boundingBox())!.y);
    } finally {
      for (const c of created) await deleteTestContact(request, c.ID);
    }
  });
});
