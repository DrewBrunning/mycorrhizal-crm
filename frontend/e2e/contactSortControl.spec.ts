import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { E2E_CONTACT_PREFIX } from './global-setup';

/**
 * T77 — the
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
    await expect(page.getByRole('combobox', { name: 'Sort' })).toBeVisible();
    await expect(page.getByText('Name (A–Z)')).toBeVisible();

    // Target the combobox itself, never getByLabel('Sort'): MUI's open/closed
    // menu renders a <ul role="listbox"> that shares the same aria-labelledby
    // as the select, so the label resolves to two elements while a menu is
    // mounted, and the contact-selection checkboxes' "Select …" labels are
    // substring-matched by "Sort" once any list contains a name with it
    // (e.g. the T77Sort fixtures). Both tripped strict mode.
    await page.getByRole('combobox', { name: 'Sort' }).click();
    await page.getByRole('option', { name: 'Recently edited (newest first)' }).click();
    await expect(page).toHaveURL(/sort=updated_at/);
    await expect(page).toHaveURL(/order=desc/);

    await page.getByRole('combobox', { name: 'Sort' }).click();
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
      // T103: the contact-info filter defaults on, and these sort fixtures
      // carry no email/phone/URL — opt out explicitly so the spec keeps
      // testing the sort control, not the filter.
      await page.goto(`/contacts?search=${encodeURIComponent(search)}&has_contact_info=false`);
      await waitForLoading(page);

      // The three contacts are the only cards on the page (search isolates
      // them); the name sort orders them by sort_name, so the first card tells
      // us the current direction. Assert on the first card so the check
      // auto-retries through the refetch the sort change triggers.
      const firstCard = page.locator('.MuiCard-root').first();

      // Default name ascending: T77Alpha first.
      await expect(firstCard).toContainText('T77Alpha');

      // Flip to descending: T77Zulu moves to the top.
      await page.getByRole('combobox', { name: 'Sort' }).click();
      await page.getByRole('option', { name: 'Name (Z–A)' }).click();
      await expect(firstCard).toContainText('T77Zulu');
    } finally {
      for (const c of created) await deleteTestContact(request, c.ID);
    }
  });
});
