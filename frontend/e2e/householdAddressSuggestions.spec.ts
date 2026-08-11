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
    // First names are made unique per run so a leftover household from a
    // crashed earlier run can never collide with this run's assertions (the
    // compose DB persists until `down -v`).
    const suffix = Date.now().toString();
    const a = await createTestContact(page.request, { firstname: `E2EAddrA${suffix}`, lastname: 'T40', addresses: [sharedAddress] });
    const b = await createTestContact(page.request, { firstname: `E2EAddrB${suffix}`, lastname: 'T40', addresses: [sharedAddress] });

    try {
      await page.goto('/households');
      await page.getByRole('button', { name: /suggest households/i }).click();

      // The suggestion card renders the shared address and both members.
      await expect(
        page.getByText('742 Evergreen Terrace, Springfield, IL, 62704')
      ).toBeVisible();
      await expect(page.getByText(new RegExp(`${a.firstname}`))).toBeVisible();
      await expect(page.getByText(new RegExp(`${b.firstname}`))).toBeVisible();

      // Accepting materializes a Household whose generated name joins the
      // members' first names. Member order is deterministic but derived from
      // the members' UUIDs (server-side sort), so match either order.
      await page.getByRole('button', { name: 'Accept' }).click();
      const eitherOrder = new RegExp(`(${a.firstname}.*${b.firstname}|${b.firstname}.*${a.firstname})`);
      await expect(page.getByText(eitherOrder)).toBeVisible();

      // The suggestion is gone from the list, and the new household persists
      // after reload.
      await expect(page.getByText('742 Evergreen Terrace, Springfield, IL, 62704')).toBeHidden();
      await page.reload();
      await expect(page.getByText(eitherOrder)).toBeVisible();
    } finally {
      await deleteTestContact(page.request, a.ID);
      await deleteTestContact(page.request, b.ID);
    }
  });
});
