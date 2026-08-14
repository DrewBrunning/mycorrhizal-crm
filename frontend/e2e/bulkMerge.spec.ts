import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading, stableClick } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Bulk merge from the contacts list (T92 — docs/fork-plan/tickets/
 * 136-T92-bulk-merge-from-contacts-list.md). Merge is pairwise, so the
 * Contacts page's bulk bar only enables it at exactly two selected rows:
 * anything else is disabled with an explanatory tooltip. Selecting two opens
 * MergeContactsDialog in pair mode (neither row is privileged — the user
 * picks the keeper), and a successful merge must refetch the list and clear
 * the selection so the dead loser row can't linger.
 *
 * The merge pair shares a last name + email but has different first names, so
 * the preview surfaces a real firstname conflict the user must resolve before
 * the commit enables — deliberately exercising the conflict UI, not just the
 * no-conflict fast path. Contacts are seeded with E2E_CONTACT_PREFIX so the
 * global-setup sweep can reclaim any that leak on a mid-run crash, and the
 * list is scoped with `?search=` so the spec never depends on whatever else
 * is in the shared test account.
 */
test.describe('Bulk merge from the contacts list', () => {
  test('enables only at two selected and merges the pair, refreshing the list', async ({ page, request }) => {
    const runId = `BulkMerge${Date.now()}`;
    const sharedEmail = `bulkmerge-${runId}@example.com`;
    const keeperName = `${E2E_CONTACT_PREFIX}${runId}Keeper`;
    const loserName = `${E2E_CONTACT_PREFIX}${runId}Loser`;

    const keeper = await createTestContact(request, { firstname: keeperName, lastname: 'Scan', email: sharedEmail });
    const loser = await createTestContact(request, { firstname: loserName, lastname: 'Scan', email: sharedEmail });

    try {
      await page.goto(`/contacts?search=${encodeURIComponent(runId)}`);
      await waitForLoading(page);

      const keeperLabel = `Select ${keeperName} Scan`;
      const loserLabel = `Select ${loserName} Scan`;
      await expect(page.getByLabel(keeperLabel)).toBeVisible({ timeout: 10000 });
      await expect(page.getByLabel(loserLabel)).toBeVisible();

      // One selected: Merge is disabled with the constraint explained.
      await page.getByLabel(keeperLabel).click();
      await expect(page.getByText('1 selected')).toBeVisible();
      const bulkMergeButton = page.getByRole('button', { name: /^merge$/i });
      await expect(bulkMergeButton).toBeDisabled();
      // force: the opened tooltip sits over the button, so Playwright's own
      // actionability check (nothing intercepts the pointer) can never pass —
      // the mouse still lands, which is what opens the tooltip.
      await bulkMergeButton.hover({ force: true });
      await expect(page.getByText('Select exactly two contacts to merge.')).toBeVisible();

      // Two selected: Merge enables and opens the pair-mode dialog.
      await page.getByLabel(loserLabel).click();
      await expect(page.getByText('2 selected')).toBeVisible();
      await expect(bulkMergeButton).toBeEnabled();

      const mergeDialog = page.getByRole('dialog').filter({ hasText: 'Merge Contacts' });
      await stableClick(bulkMergeButton);
      await expect(mergeDialog).toBeVisible({ timeout: 10000 });
      // Pair mode: neither selected row is privileged — both are offered as
      // keeper candidates.
      await expect(mergeDialog.getByText(`Keep ${keeperName} Scan`)).toBeVisible();
      await expect(mergeDialog.getByText(`Keep ${loserName} Scan`)).toBeVisible();

      // The distinct first names are a real scalar conflict the user must
      // resolve before the commit enables. Keep the keeper's first name.
      await mergeDialog.getByRole('radio', { name: /Keeper Scan:/ }).click();

      // With the conflict resolved, the dialog's Merge enables. Scope inside
      // the dialog: the bulk bar's own Merge button is still in the DOM
      // behind it.
      const dialogMerge = mergeDialog.getByRole('button', { name: /^Merge$/ });
      await expect(dialogMerge).toBeEnabled({ timeout: 10000 });
      await stableClick(dialogMerge);

      // The dialog only closes on a successful commit, and only then does the
      // parent refetch and clear the selection — so by the time it resolves
      // the loser is gone from the list and the selection banner is empty.
      await expect(mergeDialog).not.toBeVisible({ timeout: 10000 });
      await expect(page.getByText(/\d+ selected/)).not.toBeVisible();
      await expect(page.getByText(`${loserName} Scan`)).not.toBeVisible({ timeout: 10000 });
      await expect(page.getByText(`${keeperName} Scan`)).toBeVisible();

      // The loser must no longer be retrievable. The merge commit and a
      // follow-up GET can race through SQLite's pooled connections (a WAL
      // read snapshot), so poll briefly rather than asserting a single
      // immediate request — same caveat as T93's spec.
      await expect
        .poll(async () => (await request.get(`${API_BASE_URL}/contacts/${loser.ID}`)).status(), {
          message: 'the merged-away contact must no longer be retrievable',
          timeout: 10000,
        })
        .toBe(404);
      const keeperLookup = await request.get(`${API_BASE_URL}/contacts/${keeper.ID}`);
      expect(keeperLookup.ok(), 'the keeper must survive the merge').toBeTruthy();
    } finally {
      await deleteTestContact(request, keeper.ID);
      await deleteTestContact(request, loser.ID);
    }
  });
});
