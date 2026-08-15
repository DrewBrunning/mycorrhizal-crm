import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

// N7: a contact's attachments
// section supports upload, list, and delete through the real UI.
test.describe('Contact attachments', () => {
  test('uploads an attachment and deletes it', async ({ page, request }) => {
    const contact = await createTestContact(page.request, { firstname: 'E2E-Attach', lastname: `N7${Date.now()}` });

    try {
      // Stub confirm to true (before navigation) so the delete confirmation
      // dialog is deterministic.
      await page.addInitScript(() => {
        window.confirm = () => true;
      });

      await page.goto(`/contacts/${contact.ID}`);
      await expect(page.getByRole('heading', { name: 'Attachments' })).toBeVisible();

      // Upload a real file through the hidden input in the attachments section.
      const fileInput = page.locator('#attachments input[type="file"]');
      await fileInput.setInputFiles({
        name: 'insurance-scan.pdf',
        mimeType: 'application/pdf',
        buffer: Buffer.from('%PDF-1.4 fake insurance scan'),
      });

      // The uploaded file appears in the list.
      await expect(page.getByText('insurance-scan.pdf')).toBeVisible();
      await expect(page.getByText(/application\/pdf/)).toBeVisible();

      // The attachment is persisted server-side (listable via the API).
      const list = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/attachments`);
      expect(list.ok()).toBeTruthy();
      const body = await list.json();
      expect(body.total).toBe(1);

      // Deleting it removes it from the list.
      await page.locator('#attachments').getByLabel('Delete').first().click();
      await expect(page.getByText('insurance-scan.pdf')).toBeHidden();

      const after = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/attachments`);
      const afterBody = await after.json();
      expect(afterBody.total).toBe(0);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
