import { createTestContact, expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

// Issue #2002: the contact Trash is the restore/purge UI for soft-deleted
// contacts. The unit tests pin the page's behaviour against a mocked API; this
// spec drives the real stack end to end (delete -> list -> restore/purge/empty)
// so the route, the snapshot-backed backend, and the page stay wired together.

/** Soft-deletes (if still live) and then purges so a failed assertion cannot leak a trashed row. */
async function purgeTestContact(
  request: Parameters<typeof createTestContact>[0],
  id: number,
): Promise<void> {
  await request.delete(`${API_BASE_URL}/contacts/${id}`).catch(() => {});
  await request.delete(`${API_BASE_URL}/trash/contacts/${id}`).catch(() => {});
}

function uniqueName(prefix: string): string {
  return `E2E-Trash${prefix}${Date.now()}${Math.floor(Math.random() * 1000)}`;
}

test.describe('Contact trash', () => {
  test('deleting through the UI moves the contact to the Trash and Restore brings it back', async ({
    page,
  }) => {
    const name = uniqueName('Restore');
    const contact = await createTestContact(page.request, { firstname: name, lastname: 'Trash' });
    try {
      // The delete confirmation is a window.confirm; stub it before navigation.
      await page.addInitScript(() => {
        window.confirm = () => true;
      });

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      await expect(page.getByRole('heading', { name: `${name} Trash` })).toBeVisible();

      // Delete is the last item in the header's More menu at the default desktop width.
      await page.getByRole('button', { name: 'More' }).click();
      await page.getByRole('menuitem', { name: 'Delete Contact' }).click();

      // The backend soft-deletes it into the trash.
      await expect
        .poll(async () =>
          (await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`)).status(),
        )
        .toBe(404);

      await page.goto('/trash');
      const row = page.getByRole('listitem').filter({ hasText: name });
      await expect(row).toBeVisible();
      await expect(row).toContainText('Deleted ');

      await row.getByRole('button', { name: 'Restore' }).click();
      await expect(row).toHaveCount(0);

      // Restored to the live set, not just removed from the list.
      const live = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      expect(live.ok()).toBeTruthy();
    } finally {
      await purgeTestContact(page.request, contact.ID);
    }
  });

  test('Delete forever purges one contact from the Trash', async ({ page }) => {
    const name = uniqueName('Purge');
    const contact = await createTestContact(page.request, { firstname: name, lastname: 'Trash' });
    try {
      const deleted = await page.request.delete(`${API_BASE_URL}/contacts/${contact.ID}`);
      expect(deleted.ok()).toBeTruthy();

      await page.addInitScript(() => {
        window.confirm = () => true;
      });
      await page.goto('/trash');

      const row = page.getByRole('listitem').filter({ hasText: name });
      await expect(row).toBeVisible();
      await row.getByRole('button', { name: 'Delete forever' }).click();
      await expect(row).toHaveCount(0);

      const gone = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      expect(gone.status()).toBe(404);
      const trash = await page.request.get(`${API_BASE_URL}/trash/contacts?limit=200`);
      expect(trash.ok()).toBeTruthy();
      const body = await trash.json();
      expect(
        body.contacts.some((c: { contact: { id: number } }) => c.contact.id === contact.ID),
      ).toBe(false);
    } finally {
      await purgeTestContact(page.request, contact.ID);
    }
  });

  test('Empty trash purges every trashed contact for the user', async ({ page }) => {
    const first = await createTestContact(page.request, {
      firstname: uniqueName('EmptyA'),
      lastname: 'Trash',
    });
    const second = await createTestContact(page.request, {
      firstname: uniqueName('EmptyB'),
      lastname: 'Trash',
    });
    try {
      for (const contact of [first, second]) {
        const deleted = await page.request.delete(`${API_BASE_URL}/contacts/${contact.ID}`);
        expect(deleted.ok()).toBeTruthy();
      }

      await page.addInitScript(() => {
        window.confirm = () => true;
      });
      await page.goto('/trash');
      await expect(page.getByRole('listitem').filter({ hasText: first.firstname })).toBeVisible();
      await expect(page.getByRole('listitem').filter({ hasText: second.firstname })).toBeVisible();

      await page.getByRole('button', { name: 'Empty trash' }).click();
      await expect(page.getByText(/Trash is empty/)).toBeVisible();

      for (const contact of [first, second]) {
        const gone = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
        expect(gone.status()).toBe(404);
      }
    } finally {
      await purgeTestContact(page.request, first.ID);
      await purgeTestContact(page.request, second.ID);
    }
  });
});
