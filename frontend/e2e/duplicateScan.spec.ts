import {
  createTestContact,
  deleteTestContact,
  expect,
  stableClick,
  test,
  waitForLoading,
} from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Duplicate scan (T93 — T93). The review surface's contract is exactly the kind
 * of thing a unit test can't see: that the pairs the dialog lists match what
 * the scan endpoint returns, that a dismissal actually persists across dialog
 * reopenings, and that Merge opens the pair merge flow against the right two
 * contacts.
 *
 * Contacts are seeded with names starting with E2E_CONTACT_PREFIX so the
 * global-setup sweep can reclaim any that leak on a mid-run crash.
 */
test.describe('Duplicate scan', () => {
  test('lists pairs, dismisses one persistently, and merges another', async ({ page, request }) => {
    const runId = `Dup${Date.now()}`;
    const sharedEmail = `dup-${runId}@example.com`;
    const mergeName = `${E2E_CONTACT_PREFIX}${runId}MergeName`;

    // Merge pair: identical names + identical email -> matched by both the
    // email and name tiers, and (critically for the UI flow) no scalar
    // conflicts to resolve, so the merge commits immediately.
    const keeper = await createTestContact(request, {
      firstname: mergeName,
      lastname: 'Scan',
      email: sharedEmail,
    });
    const loser = await createTestContact(request, {
      firstname: mergeName,
      lastname: 'Scan',
      email: sharedEmail,
    });

    // Dismiss pair: different names, same number in different formats (the
    // T68 country-code/punctuation case) -> matched by the phone tier only.
    const phoneA = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}${runId}PhoneA`,
      lastname: 'Scan',
      phones: [{ value: '+18005551234' }],
    });
    const phoneB = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}${runId}PhoneB`,
      lastname: 'Scan',
      phones: [{ value: '(800) 555-1234' }],
    });

    try {
      // --- API contract: the scan returns both pairs with the right reasons.
      // The scan is a read that can briefly miss the just-created contacts
      // through a stale WAL read snapshot, so poll until both pairs surface.
      const hasPair = (pairs: { a: { uid: string }; b: { uid: string } }[], uids: string[]) =>
        pairs.some(
          (p) =>
            (p.a.uid === uids[0] && p.b.uid === uids[1]) ||
            (p.a.uid === uids[1] && p.b.uid === uids[0]),
        );
      await expect
        .poll(
          async () => {
            const res = await request.get(`${API_BASE_URL}/contacts/duplicates`);
            if (!res.ok()) throw new Error(`scan failed: ${await res.text()}`);
            const pairs = (await res.json()).pairs as { a: { uid: string }; b: { uid: string } }[];
            return (
              hasPair(pairs, [keeper.uid, loser.uid]) && hasPair(pairs, [phoneA.uid, phoneB.uid])
            );
          },
          {
            message: 'scan must detect the same-email and country-code phone pairs',
            timeout: 10000,
          },
        )
        .toBe(true);

      // --- UI: open the review dialog from the Contacts page
      await page.goto('/contacts');
      await waitForLoading(page);
      await stableClick(page.getByRole('button', { name: 'Review duplicates' }));
      // The dialog sits over the contacts list, which contains the same
      // contact names — scope every assertion inside the dialog.
      const reviewDialog = page.getByRole('dialog').filter({ hasText: 'Review duplicates' });
      await expect(reviewDialog.getByRole('heading', { name: 'Review duplicates' })).toBeVisible({
        timeout: 10000,
      });
      await expect(reviewDialog.getByText(`${mergeName} Scan`).first()).toBeVisible();
      await expect(
        reviewDialog.getByText(`${E2E_CONTACT_PREFIX}${runId}PhoneA Scan`),
      ).toBeVisible();

      // --- Dismiss the phone pair through the UI (accept the confirm dialog)
      const phoneRow = reviewDialog
        .locator('div')
        .filter({
          has: page.getByText(`${E2E_CONTACT_PREFIX}${runId}PhoneA Scan`, { exact: true }),
        })
        .filter({
          has: page.getByText(`${E2E_CONTACT_PREFIX}${runId}PhoneB Scan`, { exact: true }),
        })
        .filter({ has: page.getByRole('button', { name: 'Not a duplicate' }) })
        .last();
      const dismissPhone = phoneRow.getByRole('button', { name: 'Not a duplicate' });

      let dialogMessage = '';
      page.once('dialog', async (dialog) => {
        dialogMessage = dialog.message();
        await dialog.accept();
      });
      await stableClick(dismissPhone);
      expect(dialogMessage).toContain('NOT duplicates');

      await expect(
        reviewDialog.getByText(`${E2E_CONTACT_PREFIX}${runId}PhoneA Scan`),
      ).not.toBeVisible({ timeout: 10000 });

      // The dismissal is a server-side fact: the scan endpoint agrees. The
      // dismissal write and a follow-up scan can race through the WAL read
      // snapshot, so poll until the pair drops out.
      await expect
        .poll(
          async () => {
            const res = await request.get(`${API_BASE_URL}/contacts/duplicates`);
            if (!res.ok()) throw new Error(`scan failed: ${await res.text()}`);
            const pairs = (await res.json()).pairs as { a: { uid: string }; b: { uid: string } }[];
            return pairs.some(
              (p) =>
                (p.a.uid === phoneA.uid && p.b.uid === phoneB.uid) ||
                (p.a.uid === phoneB.uid && p.b.uid === phoneA.uid),
            );
          },
          { message: 'a dismissed pair must not be offered again by the scan', timeout: 10000 },
        )
        .toBe(false);

      // --- Persistence: reopen the dialog and the pair stays gone
      await reviewDialog.getByRole('button', { name: 'Close' }).click();
      await stableClick(page.getByRole('button', { name: 'Review duplicates' }));
      const reopenedDialog = page.getByRole('dialog').filter({ hasText: 'Review duplicates' });
      await expect(reopenedDialog.getByText(`${mergeName} Scan`).first()).toBeVisible({
        timeout: 10000,
      });
      await expect(
        reopenedDialog.getByText(`${E2E_CONTACT_PREFIX}${runId}PhoneA Scan`),
      ).not.toBeVisible();

      // --- Merge the same-name pair through the review dialog
      const mergeRow = reopenedDialog
        .locator('div')
        .filter({ has: page.getByText(`${mergeName} Scan`, { exact: true }) })
        .filter({ has: page.getByRole('button', { name: 'Merge' }) })
        .last();
      await stableClick(mergeRow.getByRole('button', { name: 'Merge' }));

      // MergeContactsDialog opens in pair mode; identical names mean no
      // conflicts, so the Merge button enables as soon as the preview lands.
      // Scope to the merge dialog: the review dialog's own row-Merge buttons
      // are still in the DOM behind it.
      const mergeDialog = page.getByRole('dialog').filter({ hasText: 'Merge Contacts' });
      await expect(mergeDialog).toBeVisible({ timeout: 10000 });
      const mergeButton = mergeDialog.getByRole('button', { name: /^Merge$/ });
      await expect(mergeButton).toBeEnabled({ timeout: 10000 });
      await stableClick(mergeButton);

      // The merge dialog only closes on a successful commit, and only then
      // does the review list refetch — so the loser is gone from the list by
      // the time this resolves.
      await expect(mergeDialog).not.toBeVisible({ timeout: 10000 });
      await expect(reopenedDialog.getByText(`${mergeName} Scan`).first()).not.toBeVisible({
        timeout: 10000,
      });

      // The loser must no longer be retrievable. The merge commit and a
      // follow-up GET can race through SQLite's pooled connections (a WAL
      // read snapshot), so poll briefly rather than asserting a single
      // immediate request.
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
      await deleteTestContact(request, phoneA.ID);
      await deleteTestContact(request, phoneB.ID);
    }
  });
});
