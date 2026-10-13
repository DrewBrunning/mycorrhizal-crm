import { createTestContact, deleteTestContact, expect, test } from './fixtures';

// ADR 0008 / issue #1656: the contact detail page sends the loaded `revision`
// as `If-Match`, so the second of two tabs to save gets a conflict dialog
// instead of silently overwriting the first tab's edit.
test.describe('Concurrent contact edit conflict', () => {
  test('the second tab to save sees the conflict dialog and can load the latest version', async ({
    page,
    context,
  }) => {
    const contact = await createTestContact(page.request, { lastname: 'Original' });
    const pageB = await context.newPage();

    async function saveLastName(p: typeof page, lastName: string) {
      await p.locator('.edit-icon').first().click();
      await p.getByLabel('Last Name', { exact: true }).fill(lastName);
      await p.getByRole('button', { name: 'Save' }).click();
    }

    try {
      // Both tabs load the same revision.
      await page.goto(`/contacts/${contact.ID}`);
      await pageB.goto(`/contacts/${contact.ID}`);
      await expect(
        page.getByRole('heading', { name: `${contact.firstname} Original` }),
      ).toBeVisible();
      await expect(
        pageB.getByRole('heading', { name: `${contact.firstname} Original` }),
      ).toBeVisible();

      // Tab A saves first: succeeds.
      await saveLastName(page, 'FromTabA');
      await expect(
        page.getByRole('heading', { name: `${contact.firstname} FromTabA` }),
      ).toBeVisible();

      // Tab B still holds the old revision: its save is rejected with a dialog.
      await saveLastName(pageB, 'FromTabB');
      const dialog = pageB.getByRole('dialog').filter({ hasText: 'Changed elsewhere' });
      await expect(dialog).toBeVisible();

      // Reload latest: discards B's edit and shows A's save.
      await dialog.getByRole('button', { name: 'Load latest version' }).click();
      await expect(dialog).toBeHidden();
      await expect(
        pageB.getByRole('heading', { name: `${contact.firstname} FromTabA` }),
      ).toBeVisible();

      // B can now save against the fresh revision.
      await saveLastName(pageB, 'FromTabBAgain');
      await expect(
        pageB.getByRole('heading', { name: `${contact.firstname} FromTabBAgain` }),
      ).toBeVisible();
    } finally {
      await pageB.close();
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
