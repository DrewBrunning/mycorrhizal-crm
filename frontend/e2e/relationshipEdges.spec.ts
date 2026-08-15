import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

// §3d WP3 : replaces
// relationships.spec.ts now that the Relationships tab talks to
// RelationshipEdge instead of the legacy models.Relationship. Accept/Reject
// (the suggestion-review flow) is intentionally not covered here -- nothing
// in this app can produce a `status: suggested` edge yet
// (services/household_service.go's GenerateHouseholdSuggestions has no HTTP
// trigger anywhere), so there's no real data this e2e run could exercise
// that path against. See RelationshipEdgeList.test.tsx for that coverage
// instead, via a mocked suggested-status edge.
test.describe('RelationshipEdges', () => {
  test('add a manual (thin-contact) relationship to a contact', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const relName = `E2E Rel ${Date.now()}`;

    try {
      await page.goto(`/contacts/${contact.ID}`);

      await page.getByRole('button', { name: /add relationship/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      // Manual entry is the default mode.
      await dialog.getByRole('textbox', { name: /^name/i }).fill(relName);

      // The Relationship Type Select is the first combobox in this dialog
      // (Gender is second, manual-mode only; Sensitivity is third).
      await dialog.getByRole('combobox').first().click();
      await page.getByRole('option', { name: 'Friend', exact: true }).click();

      await dialog.getByRole('button', { name: /^save$/i }).click();

      await expect(dialog).toBeHidden();
      await expect(page.getByText(relName)).toBeVisible();
      await expect(page.getByText('Friend', { exact: true })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('add a linked-contact relationship, visible with the inverse label from the other contact\'s page', async ({ page }) => {
    const alice = await createTestContact(page.request, { firstname: 'E2E-Alice', lastname: `Rel${Date.now()}` });
    const bob = await createTestContact(page.request, { firstname: 'E2E-Bob', lastname: `Rel${Date.now()}` });

    try {
      await page.goto(`/contacts/${alice.ID}`);
      await page.getByRole('button', { name: /add relationship/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      // Switch to linked-contact mode and select Bob.
      await dialog.getByRole('radio', { name: /link to existing contact/i }).click();
      const autocomplete = dialog.getByRole('combobox', { name: /select contact/i });
      await autocomplete.fill(bob.lastname);
      await page.getByRole('option', { name: new RegExp(bob.lastname) }).click();

      // In linked mode the DOM order is: Autocomplete (combobox #0), then
      // the Relationship Type Select (combobox #1) -- no Gender field shown.
      await dialog.getByRole('combobox').nth(1).click();
      await page.getByRole('option', { name: 'Parent', exact: true }).click();

      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();

      // From Alice's page: Bob is her Parent. `exact: true` is required since
      // T31 renders the card's RelatedToMembers metadata on the same page
      // ("urn:uuid:... (parent_of)") — the edge card's type label is the only
      // exact "Parent" text.
      await expect(page.getByText('E2E-Bob')).toBeVisible();
      await expect(page.getByText('Parent', { exact: true })).toBeVisible();

      // From Bob's page: Alice is his Child (the inverse label).
      await page.goto(`/contacts/${bob.ID}`);
      await expect(page.getByText('E2E-Alice')).toBeVisible();
      await expect(page.getByText('Child', { exact: true })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, alice.ID);
      await deleteTestContact(page.request, bob.ID);
    }
  });

  test('edit a relationship\'s type', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const relName = `E2E Edit ${Date.now()}`;

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await page.getByRole('button', { name: /add relationship/i }).click();

      const createDialog = page.getByRole('dialog');
      await createDialog.getByRole('textbox', { name: /^name/i }).fill(relName);
      await createDialog.getByRole('combobox').first().click();
      await page.getByRole('option', { name: 'Friend', exact: true }).click();
      await createDialog.getByRole('button', { name: /^save$/i }).click();
      await expect(createDialog).toBeHidden();

      // Hover the card to reveal the edit action, then edit the type.
      const card = page.locator('text=' + relName).locator('..').locator('..');
      await card.hover();
      await card.getByLabel('Edit').click();

      const editDialog = page.getByRole('dialog');
      await expect(editDialog).toBeVisible();
      await editDialog.getByRole('combobox').first().click();
      await page.getByRole('option', { name: 'Sibling', exact: true }).click();
      await editDialog.getByRole('button', { name: /^save$/i }).click();
      // Extra headroom under parallel-worker write contention (confirmed via
      // a real run: passes reliably alone, flaked once under 2 workers) —
      // see the identical note in timeline.spec.ts.
      await expect(editDialog).toBeHidden({ timeout: 10000 });

      await expect(page.getByText('Sibling', { exact: true })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  // T37:
  // creating a pet via the "enter manually" path must default the new
  // contact's CRM.Kind to animal, so the household engine doesn't treat it as
  // a human adult. Verified end-to-end: create the pet from a human contact's
  // page (the frontend sends source_thin on an owned_by edge), then open the
  // pet's own detail page and confirm the animal badge (T27's UI) renders.
  test('a contact created as a pet relationship is labelled as an animal', async ({ page, request }) => {
    const owner = await createTestContact(page.request, { firstname: 'E2E-Pet-Owner', lastname: `Rel${Date.now()}` });
    const petName = `E2E-Pet ${Date.now()}`;

    try {
      await page.goto(`/contacts/${owner.ID}`);
      await page.getByRole('button', { name: /add relationship/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();

      await dialog.getByRole('textbox', { name: /^name/i }).fill(petName);
      await dialog.getByRole('combobox').first().click();
      await page.getByRole('option', { name: 'Pet', exact: true }).click();
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();
      await expect(page.getByText(petName)).toBeVisible();

      // Resolve the pet's numeric contact ID via search so we can open its
      // detail page (the edge response only carries the pet's vcard UID).
      const searchResponse = await request.get(
        `${API_BASE_URL}/search?q=${encodeURIComponent(petName)}&limit=10`
      );
      expect(searchResponse.ok(), `search for pet failed: ${searchResponse.status()}`).toBeTruthy();
      const searchBody = await searchResponse.json();
      const pet = searchBody.contacts.find((c: { firstname: string }) => c.firstname === petName);
      expect(pet, 'pet contact should be findable by name').toBeTruthy();

      await page.goto(`/contacts/${pet.id}`);
      await expect(page.getByText('Animal', { exact: true })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, owner.ID);
    }
  });

  test('delete a relationship', async ({ page }) => {
    const contact = await createTestContact(page.request);
    const relName = `E2E Delete ${Date.now()}`;

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await page.getByRole('button', { name: /add relationship/i }).click();

      const dialog = page.getByRole('dialog');
      await dialog.getByRole('textbox', { name: /^name/i }).fill(relName);
      await dialog.getByRole('combobox').first().click();
      await page.getByRole('option', { name: 'Friend', exact: true }).click();
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();
      await expect(page.getByText(relName)).toBeVisible();

      page.on('dialog', (d) => d.accept());
      const card = page.locator('text=' + relName).locator('..').locator('..');
      await card.hover();
      await card.getByLabel('Delete').click();

      await expect(page.getByText(relName)).toBeHidden();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
