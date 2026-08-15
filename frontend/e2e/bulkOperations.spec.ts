import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

// N5 bulk operations. Uses
// a `?search=` filter to scope the contacts list to exactly this test's own
// throwaway contacts, rather than relying on "select all" over whatever else
// happens to be seeded/leftover in the shared test account -- the list page
// size is 10, so 11 unique contacts reliably span two pages.
test.describe('Bulk operations', () => {
  test('select across two pages, bulk-tag, then bulk-delete with confirmation', async ({ page }) => {
    const runId = `BulkE2E${Date.now()}`;
    const contacts = [];
    for (let i = 0; i < 11; i++) {
      contacts.push(await createTestContact(page.request, { lastname: `${runId}-${i}` }));
    }

    const tagRes = await page.request.post(`${API_BASE_URL}/tags`, { data: { name: `e2e-bulk-tag-${Date.now()}` } });
    expect(tagRes.ok(), `failed to create test tag: ${tagRes.status()}`).toBeTruthy();
    const tag = (await tagRes.json()).tag;

    try {
      // T103: the contact-info filter defaults on, and these throwaway bulk
      // contacts carry no email/phone/URL — opt out explicitly so the spec
      // keeps testing pagination/selection, not the filter.
      await page.goto(`/contacts?search=${encodeURIComponent(runId)}&has_contact_info=false`);
      await waitForLoading(page);

      // Select all of page one (10 of the 11).
      await page.getByLabel(/^select all$/i).click();
      await expect(page.getByText('10 selected')).toBeVisible();

      // Load page two and select-all again to cover the boundary.
      await page.getByRole('button', { name: /load more/i }).click();
      await waitForLoading(page);
      await page.getByLabel(/^select all$/i).click();
      await expect(page.getByText('11 selected')).toBeVisible();

      // Bulk-add the tag to all 11.
      await page.getByLabel(/^tag…$/i).click();
      await page.getByRole('option', { name: tag.name, exact: true }).click();
      await page.getByRole('button', { name: /^add tag$/i }).click();

      // Success clears the selection banner (no failure alert to dismiss).
      await expect(page.getByText(/\d+ selected/)).not.toBeVisible();

      const tagCheck = await page.request.get(`${API_BASE_URL}/tags/${tag.id}`);
      expect(tagCheck.ok()).toBeTruthy();
      const tagged = (await tagCheck.json()).contacts as { contact_vcard_uid: string }[];
      const taggedUids = new Set(tagged.map((row) => row.contact_vcard_uid));
      for (const c of contacts) {
        expect(taggedUids.has(c.uid), `${c.uid} should have been tagged`).toBe(true);
      }

      // Bulk delete: select all again, confirm the dialog names the count,
      // then accept it -- the most destructive bulk action in the app.
      await page.getByLabel(/^select all$/i).click();
      await expect(page.getByText('10 selected')).toBeVisible();
      await page.getByRole('button', { name: /load more/i }).click();
      await waitForLoading(page);
      await page.getByLabel(/^select all$/i).click();
      await expect(page.getByText('11 selected')).toBeVisible();

      // Playwright auto-dismisses native dialogs unless a page.on('dialog')
      // listener is registered before the action that triggers one -- a
      // waitForEvent registered after .click() races the auto-dismiss and
      // hangs, since the click doesn't resolve until the confirm() call
      // (which the auto-dismiss races) returns.
      let dialogMessage = '';
      page.once('dialog', async (dialog) => {
        dialogMessage = dialog.message();
        await dialog.accept();
      });
      await page.getByRole('button', { name: /^delete$/i }).click();
      await expect(page.getByText(/\d+ selected/)).not.toBeVisible();
      expect(dialogMessage).toContain('Delete 11 contacts?');

      await expect(page.getByText(/\d+ selected/)).not.toBeVisible();

      // Each contact must be gone. The delete commit and a follow-up GET can
      // race through SQLite's pooled connections (a WAL read snapshot), so
      // poll briefly rather than asserting a single immediate request.
      for (const c of contacts) {
        await expect
          .poll(async () => (await page.request.get(`${API_BASE_URL}/contacts/${c.ID}`)).status(), {
            message: `contact ${c.ID} should be gone after bulk delete`,
            timeout: 10000,
          })
          .toBe(404);
      }
    } finally {
      await page.request.delete(`${API_BASE_URL}/tags/${tag.id}`).catch(() => {});
      for (const c of contacts) {
        await deleteTestContact(page.request, c.ID);
      }
    }
  });
});
