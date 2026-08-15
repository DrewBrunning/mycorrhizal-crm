import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

// T104 + address suggestions: the "propose data" surface on the Data settings
// page. Two engines:
//   - "Suggest relationships" runs one round of two-hop graph inference over
//     confirmed edges and lands the results in the global suggestion inbox.
//   - "Suggest addresses" proposes addresses a contact probably shares, from a
//     confirmed relationship or household membership, and each row is applied
//     explicitly.
// Both are scoped to this test's own throwaway contacts so the shared
// account's other data can never confuse the assertions.
test.describe('Data suggestions (propose data)', () => {
  test('suggests a relationship from relationships and accepting it confirms it', async ({ page, request }) => {
    const suffix = Date.now().toString();
    const parent = await createTestContact(page.request, { firstname: `E2ESugParent${suffix}`, lastname: 'T104' });
    const child1 = await createTestContact(page.request, { firstname: `E2ESugChild1${suffix}`, lastname: 'T104' });
    const child2 = await createTestContact(page.request, { firstname: `E2ESugChild2${suffix}`, lastname: 'T104' });

    try {
      // Seed the two confirmed edges the rule R2 (parent · sibling -> parent_of)
      // composes: parent is child1's parent, child1 and child2 are siblings.
      for (const [source, target, type] of [
        [parent.uid, child1.uid, 'parent_of'],
        [child1.uid, child2.uid, 'sibling_of'],
      ] as const) {
        const edge = await request.post(`${API_BASE_URL}/relationship-edges`, {
          data: { source_id: source, target_id: target, type },
        });
        expect(edge.ok(), `seed edge: ${await edge.text()}`).toBeTruthy();
      }

      await page.goto('/settings/data');
      await page.getByRole('button', { name: /suggest relationships/i }).click();

      // The suggested edge is parent_of(parent, child2), rendered as
      // "<parent> · Parent · <child2>". The parent is the source endpoint, so
      // both the parent's and child2's unique names anchor the row.
      const rowText = new RegExp(`${parent.firstname}.*Parent.*${child2.firstname}`);
      await expect(page.getByText(rowText)).toBeVisible({ timeout: 15000 });

      // Accept the specific suggestion (scoped to child2's unique name so a
      // parallel test's edges can't be matched).
      const row = page.locator('.MuiPaper-outlined').filter({ hasText: child2.firstname }).filter({ hasText: 'Parent' });
      await row.getByRole('button', { name: 'Accept' }).click();

      // Accepting promotes the edge to confirmed, so it leaves the suggested
      // inbox (and the review row disappears).
      await expect(page.getByText(rowText)).toBeHidden({ timeout: 10000 });

      // The accepted edge is now a confirmed fact, visible from child2's page.
      await page.goto(`/contacts/${child2.ID}`);
      await expect(page.getByText(parent.firstname)).toBeVisible();
      await expect(page.getByText('Parent', { exact: true })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, parent.ID);
      await deleteTestContact(page.request, child1.ID);
      await deleteTestContact(page.request, child2.ID);
    }
  });

  test('suggests a shared address and applying it writes it to the contact', async ({ page, request }) => {
    const suffix = Date.now().toString();
    const address = {
      street: `88 ${suffix} Lane`,
      city: 'Springfield',
      region: 'IL',
      postal: '62704',
      type: 'home',
    };
    const alice = await createTestContact(page.request, { firstname: `E2ESugAddrA${suffix}`, lastname: 'T167' });
    const bob = await createTestContact(page.request, {
      firstname: `E2ESugAddrB${suffix}`,
      lastname: 'T167',
      addresses: [address],
    });

    try {
      // A confirmed spouse edge implies (but doesn't guarantee) a shared
      // address, so Bob's address is proposed for Alice.
      const edge = await request.post(`${API_BASE_URL}/relationship-edges`, {
        data: { source_id: alice.uid, target_id: bob.uid, type: 'spouse_of' },
      });
      expect(edge.ok(), `seed edge: ${await edge.text()}`).toBeTruthy();

      await page.goto('/settings/data');
      // The address suggestions load on mount; click the button to (re)scan.
      await page.getByRole('button', { name: /suggest addresses/i }).click();

      const addressLine = `${address.street}, Springfield, IL, 62704`;
      // Alice's row shows her name, the proposed address, and Bob as the source.
      // `.MuiPaper-outlined` scopes to the suggestion rows (the outer "Propose
      // data" Card is `.MuiPaper-elevation`, so the ancestor can't be matched).
      const row = page.locator('.MuiPaper-outlined').filter({ hasText: alice.firstname }).filter({ hasText: addressLine });
      await expect(row).toBeVisible({ timeout: 15000 });
      await expect(row.getByText(new RegExp(bob.firstname))).toBeVisible();

      await row.getByRole('button', { name: 'Apply' }).click();

      // The applied row disappears (the contact now has the address).
      await expect(row).toBeHidden({ timeout: 10000 });

      // The address really landed: a fresh scan no longer proposes it.
      const rescan = await request.post(`${API_BASE_URL}/contacts/address-suggestions`);
      expect(rescan.ok(), `rescan: ${await rescan.text()}`).toBeTruthy();
      const body = await rescan.json();
      const stillSuggested = (body.suggestions || []).some(
        (s: { contact_vcard_uid: string }) => s.contact_vcard_uid === alice.uid
      );
      expect(stillSuggested, 'the applied address must no longer be suggested').toBe(false);
    } finally {
      await deleteTestContact(page.request, alice.ID);
      await deleteTestContact(page.request, bob.ID);
    }
  });
});
