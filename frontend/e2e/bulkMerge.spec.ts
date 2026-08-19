import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading, stableClick, selectedText } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Bulk merge from the contacts list (T92 — T92). Merge is pairwise, so the
 * Contacts page's bulk bar only enables it at exactly two selected rows:
 * anything else (one, or three-plus) is disabled with an explanatory tooltip.
 * Selecting two opens MergeContactsDialog in pair mode (neither row is
 * privileged — the user picks the keeper and can swap), and a successful merge
 * must refetch the list and clear the selection so the dead loser row can't
 * linger.
 *
 * Three contacts are seeded: the merge pair shares a last name + email but has
 * different first names, so the preview surfaces a real firstname conflict the
 * user must resolve before the commit enables — deliberately exercising the
 * conflict UI, not just the no-conflict fast path. A third throwaway contact
 * exercises the three-plus-disabled gate. All are seeded with
 * E2E_CONTACT_PREFIX so the global-setup sweep can reclaim any that leak on a
 * mid-run crash, and the list is scoped with `?search=` so the spec never
 * depends on whatever else is in the shared test account.
 */
test.describe('Bulk merge from the contacts list', () => {
  test('enables only at exactly two selected and merges the pair, refreshing the list', async ({ page, request }) => {
    const runId = `BulkMerge${Date.now()}`;
    const sharedEmail = `bulkmerge-${runId}@example.com`;
    const keeperName = `${E2E_CONTACT_PREFIX}${runId}Keeper`;
    const loserName = `${E2E_CONTACT_PREFIX}${runId}Loser`;
    const thirdName = `${E2E_CONTACT_PREFIX}${runId}Third`;

    const keeper = await createTestContact(request, { firstname: keeperName, lastname: 'Scan', email: sharedEmail });
    const loser = await createTestContact(request, { firstname: loserName, lastname: 'Scan', email: sharedEmail });
    const third = await createTestContact(request, { firstname: thirdName, lastname: 'Scan', email: sharedEmail });

    try {
      await page.goto(`/contacts?search=${encodeURIComponent(runId)}`);
      await waitForLoading(page);

      const keeperLabel = `Select ${keeperName} Scan`;
      const loserLabel = `Select ${loserName} Scan`;
      const thirdLabel = `Select ${thirdName} Scan`;
      await expect(page.getByLabel(keeperLabel)).toBeVisible({ timeout: 10000 });
      await expect(page.getByLabel(loserLabel)).toBeVisible();
      await expect(page.getByLabel(thirdLabel)).toBeVisible();

      const bulkMergeButton = page.getByRole('button', { name: /^merge$/i });

      // One selected: Merge is disabled with the constraint explained.
      await page.getByLabel(keeperLabel).click();
      await expect(selectedText(page, '1 selected')).toBeVisible();
      await expect(bulkMergeButton).toBeDisabled();
      // force: the opened tooltip sits over the button, so Playwright's own
      // actionability check (nothing intercepts the pointer) can never pass —
      // the mouse still lands, which is what opens the tooltip.
      await bulkMergeButton.hover({ force: true });
      await expect(page.getByText('Select exactly two contacts to merge.')).toBeVisible();

      // Two selected: Merge enables.
      await page.getByLabel(loserLabel).click();
      await expect(selectedText(page, '2 selected')).toBeVisible();
      await expect(bulkMergeButton).toBeEnabled();

      // Three selected: disabled again — merge is strictly pairwise.
      await page.getByLabel(thirdLabel).click();
      await expect(selectedText(page, '3 selected')).toBeVisible();
      await expect(bulkMergeButton).toBeDisabled();
      // Back to the merge pair.
      await page.getByLabel(thirdLabel).click();
      await expect(selectedText(page, '2 selected')).toBeVisible();
      await expect(bulkMergeButton).toBeEnabled();

      const mergeDialog = page.getByRole('dialog').filter({ hasText: 'Merge Contacts' });
      await stableClick(bulkMergeButton);
      await expect(mergeDialog).toBeVisible({ timeout: 10000 });
      // Pair mode: neither selected row is privileged — both are offered as
      // keeper candidates, and the default keeper is the list's first row.
      await expect(mergeDialog.getByText(`Keep ${keeperName} Scan`)).toBeVisible();
      await expect(mergeDialog.getByText(`Keep ${loserName} Scan`)).toBeVisible();
      await expect(mergeDialog.getByRole('radio', { name: `Keep ${keeperName} Scan` })).toBeChecked();

      // The swap control must actually change which contact survives.
      await mergeDialog.getByRole('button', { name: 'Swap' }).click();
      await expect(mergeDialog.getByRole('radio', { name: `Keep ${loserName} Scan` })).toBeChecked();
      // Swap back to the original keeper so the conflict resolution below
      // stays deterministic.
      await mergeDialog.getByRole('button', { name: 'Swap' }).click();
      await expect(mergeDialog.getByRole('radio', { name: `Keep ${keeperName} Scan` })).toBeChecked();

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
      await expect(selectedText(page, /\d+ selected/)).not.toBeVisible();
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
      await deleteTestContact(request, third.ID);
    }
  });
});
