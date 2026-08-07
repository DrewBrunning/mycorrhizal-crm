import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact } from './fixtures';

// T40 (docs/fork-plan/tickets/49-T40-household-suggestions-shared-address.md):
// contacts who share an address but are not in a household together are
// surfaced as a suggestion on the Households page, and accepting the
// suggestion materializes a real Household with both as members.
test.describe('Address-based household suggestions', () => {
  const sharedAddress = {
    street: '742 Evergreen Terrace',
    city: 'Springfield',
    region: 'IL',
    postal: '62704',
    type: 'home',
  };

  test('suggests a shared-address household and accepting it creates one', async ({ page }) => {
    const a = await createTestContact(page.request, { firstname: 'E2E-AddrA', lastname: `T40${Date.now()}`, addresses: [sharedAddress] });
    const b = await createTestContact(page.request, { firstname: 'E2E-AddrB', lastname: `T40${Date.now()}`, addresses: [sharedAddress] });

    try {
      await page.goto('/households');
      await page.getByRole('button', { name: /suggest households from shared address/i }).click();

      // The suggestion card renders the shared address and both members.
      await expect(
        page.getByText('742 Evergreen Terrace, Springfield, IL, 62704')
      ).toBeVisible();
      await expect(page.getByText(new RegExp(`${a.firstname}`))).toBeVisible();
      await expect(page.getByText(new RegExp(`${b.firstname}`))).toBeVisible();

      // Accepting materializes a Household whose generated name joins the
      // members' first names.
      await page.getByRole('button', { name: 'Accept' }).click();
      await expect(page.getByText(`${a.firstname} & ${b.firstname}`)).toBeVisible();

      // The suggestion is gone from the list, and the new household persists
      // after reload.
      await expect(page.getByText('742 Evergreen Terrace, Springfield, IL, 62704')).toBeHidden();
      await page.reload();
      await expect(page.getByText(`${a.firstname} & ${b.firstname}`)).toBeVisible();
    } finally {
      await deleteTestContact(page.request, a.ID);
      await deleteTestContact(page.request, b.ID);
    }
  });
});
