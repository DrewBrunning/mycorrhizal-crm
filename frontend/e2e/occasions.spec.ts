import { createTestContact, deleteTestContact, expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

// Issue #1481: Occasions (ADR 0024, issue #387) had no Playwright coverage, so
// the seam between the shipped bundle and the real server (route, auth cookie,
// the gift-shopping-list response shape) was only ever tested against mocks.
//
// A thin happy path plus one failure -- the lower layers own the matrix:
// create a gift obligation on a contact's page -> it lands on /occasions as
// "Needed" -> a purchased Gift flips it to "Purchased" -> deactivating it (the
// product's "this one is handled" switch; there is no separate done button)
// takes it off the list -> deleting the contact removes the obligation.

const MONTHS = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];

test.describe('Occasions', () => {
  test('obligation on a contact shows on /occasions, completes, and dies with the contact', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request, {
      firstname: `${E2E_CONTACT_PREFIX}Occasion`,
      lastname: 'Giftee',
    });
    const label = `E2E birthday gift ${Date.now()}`;
    // Three days out is inside the default 30-day window with slack for any
    // clock/timezone skew between this runner and the server.
    const soon = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // --- create through the real UI ---
      await page.getByRole('button', { name: 'Add occasion' }).click();
      const dialog = page.getByRole('dialog', { name: 'Add an occasion' });
      await expect(dialog).toBeVisible();
      await dialog.getByLabel('Kind').fill('gift');
      await dialog.getByLabel('Label *').fill(label);
      await dialog.getByRole('combobox', { name: 'Month' }).click();
      await page.getByRole('option', { name: MONTHS[soon.getMonth()], exact: true }).click();
      await dialog.getByRole('combobox', { name: 'Day' }).click();
      await page.getByRole('option', { name: String(soon.getDate()), exact: true }).click();
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(dialog).toBeHidden();
      await expect(page.locator('#occasions').getByText(label)).toBeVisible();

      // --- it is on the real /occasions gift shopping list, as Needed ---
      await page.goto('/occasions');
      await expect(page.getByRole('heading', { name: 'Occasions', level: 1 })).toBeVisible();
      const row = page.getByRole('link', { name: new RegExp(label) });
      await expect(row).toBeVisible();
      await expect(row).toContainText('Giftee');
      await expect(row.getByText('Needed')).toBeVisible();

      // --- a purchased gift for the contact flips the status ---
      const gift = await page.request.post(`${API_BASE_URL}/gifts`, {
        data: { entity_id: contact.uid, status: 'purchased', description: 'Scarf' },
      });
      expect(gift.ok(), `gift create: ${await gift.text()}`).toBeTruthy();
      await page.reload();
      await expect(
        page.getByRole('link', { name: new RegExp(label) }).getByText('Purchased'),
      ).toBeVisible();

      // --- deactivating the obligation takes it off the list ---
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      await page.locator('#occasions').getByRole('button', { name: 'Edit', exact: true }).click();
      const edit = page.getByRole('dialog', { name: 'Edit occasion' });
      await edit.getByRole('switch', { name: 'Active' }).uncheck();
      await edit.getByRole('button', { name: 'Save' }).click();
      await expect(edit).toBeHidden();
      await page.goto('/occasions');
      await expect(page.getByText('No gifts needed in this window.')).toBeVisible();
      await expect(page.getByRole('link', { name: new RegExp(label) })).toHaveCount(0);

      // --- failure: an invalid obligation is rejected, nothing is created ---
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      await page.getByRole('button', { name: 'Add occasion' }).click();
      const bad = page.getByRole('dialog', { name: 'Add an occasion' });
      await bad.getByLabel('Label *').fill('Half a date');
      await bad.getByRole('combobox', { name: 'Month' }).click();
      await page.getByRole('option', { name: 'January', exact: true }).click();
      await bad.getByRole('button', { name: 'Save' }).click();
      await expect(bad.getByText('Set both a month and a day, or leave both unset.')).toBeVisible();
      await bad.getByRole('button', { name: 'Cancel' }).click();

      // --- deleting the contact takes its obligation with it ---
      const before = await page.request.get(
        `${API_BASE_URL}/occasion-obligations?entity_id=${contact.uid}`,
      );
      expect((await before.json()).occasion_obligations).toHaveLength(1);
      await deleteTestContact(page.request, contact.ID);
      const after = await page.request.get(
        `${API_BASE_URL}/occasion-obligations?entity_id=${contact.uid}`,
      );
      expect((await after.json()).occasion_obligations).toHaveLength(0);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
