import { expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

// T56: the bulk
// import entry point lives in Settings → Data, alongside the export UI, and
// reuses the same wizard as the Contacts page — now with bulk controls. This
// drives the real dialog end to end through the file input.
test.describe('Bulk contacts import in Data Settings', () => {
  test('imports a CSV address book from Data Settings', async ({ page, request }) => {
    const name = `${E2E_CONTACT_PREFIX}Bulk${Date.now()}`;
    const second = `${E2E_CONTACT_PREFIX}Bulk2${Date.now()}`;
    const csv = `First Name,Last Name,Email\n${name},One,${name}@example.com\n${second},Two,${second}@example.com\n`;

    try {
      await page.goto('/settings/data');
      await waitForLoading(page);

      await page
        .getByRole('button', { name: /import contacts/i })
        .first()
        .click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      // The dialog's file input is hidden; setInputFiles works on it directly.
      await dialog.locator('#import-file-input').setInputFiles({
        name: 'addressbook.csv',
        mimeType: 'text/csv',
        buffer: Buffer.from(csv),
      });

      // CSV goes through the mapping step — accept the suggested mappings.
      await expect(dialog.getByText('Map Columns')).toBeVisible();
      await dialog.getByRole('button', { name: /continue/i }).click();

      // Preview: bulk-accept both rows, then import.
      await expect(dialog.getByText('2 to create')).toBeVisible();
      await dialog.getByRole('button', { name: /resolve all as merged/i }).click();
      await dialog.getByRole('button', { name: /apply decisions/i }).click();

      // Result step reports both created.
      await expect(dialog.getByText('2 contacts created')).toBeVisible();
      await dialog.getByRole('button', { name: /done/i }).click();
      await expect(dialog).toBeHidden();

      // The imported contacts are real: findable via the API.
      const search = await request.get(
        `${API_BASE_URL}/search?q=${encodeURIComponent(name)}&limit=10`,
      );
      expect(search.ok()).toBeTruthy();
      const body = await search.json();
      expect(
        body.contacts.some((c: { firstname: string }) => c.firstname === name),
        'the imported contact must be searchable',
      ).toBeTruthy();
    } finally {
      // Clean up the imported contacts (skip/ignore failures — the contacts
      // may not exist if the import failed mid-run). Scoped by ?search=
      // rather than an unfiltered ?limit=200 -- under a full parallel run the
      // shared account can hold well over 200 live throwaway contacts at
      // once, which would silently drop these out of an unscoped page and
      // leak them (the same cap global-setup's own leftover-sweep has, so
      // relying on that catching it isn't a given either).
      for (const firstname of [name, second]) {
        const list = await request.get(
          `${API_BASE_URL}/contacts?search=${encodeURIComponent(firstname)}&limit=10`,
        );
        const { contacts } = await list.json();
        const match = (contacts || []).find(
          (c: { firstname: string }) => c.firstname === firstname,
        );
        if (match) await request.delete(`${API_BASE_URL}/contacts/${match.id}`).catch(() => {});
      }
    }
  });
});
