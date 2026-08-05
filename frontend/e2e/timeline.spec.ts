import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';

// Notes and activities both render in the contact's Timeline section. Each
// test uses a throwaway contact and cleans it up (which cascades the
// note/activity). T31 made every panel mount eagerly — more effects run in
// parallel, so waitForLoading ensures the page is fully settled before
// interacting, and the dialog assertions get extra headroom.
test.describe('Timeline', () => {
  test('should add a note to a contact', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const noteContent = `E2E note ${Date.now()}`;

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // Timeline is always visible now (T31) — no tab to switch to.
      await page.getByRole('button', { name: /add note/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible({ timeout: 10000 });

      await dialog.getByLabel(/content/i).fill(noteContent);
      await dialog.getByRole('button', { name: /^save$/i }).click();

      await expect(dialog).toBeHidden();
      // The save awaits the create + refetch before closing the dialog, so
      // this is normally near-instant — the longer timeout is headroom for
      // SQLite write contention under parallel workers, not a masked bug.
      await expect(page.getByText(noteContent)).toBeVisible({ timeout: 10000 });
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('should add an activity to a contact', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const activityTitle = `E2E activity ${Date.now()}`;

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await page.getByRole('button', { name: /add activity/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible({ timeout: 10000 });

      await dialog.getByRole('textbox', { name: 'Title', exact: true }).fill(activityTitle);
      await dialog.getByRole('button', { name: /^save$/i }).click();

      await expect(dialog).toBeHidden();
      // See the note test above for why this gets extra headroom.
      await expect(page.getByText(activityTitle)).toBeVisible({ timeout: 10000 });
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
