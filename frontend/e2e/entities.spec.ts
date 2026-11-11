import { createTestContact, deleteTestContact, expect, test } from './fixtures';
import { API_BASE_URL } from './global-setup';

// Issue #2301 (ADR 0043): reusable organisations, places and pets. The
// automatic per-test axe scan in fixtures.ts covers the pages and dialogs this
// walks through.
test.describe('Entities', () => {
  test('link a contact as primary, see it on the entity, then delete and restore', async ({
    page,
  }) => {
    const suffix = Date.now().toString();
    const name = `E2E Acme ${suffix}`;
    const contact = await createTestContact(page.request, {
      firstname: `E2EEnt${suffix}`,
      lastname: 'Linked',
    });
    const created = await page.request.post(`${API_BASE_URL}/entities`, {
      data: { kind: 'organisation', name, core: { organization: { name } } },
    });
    expect(created.ok(), `failed to create entity: ${created.status()}`).toBeTruthy();
    const entity = await created.json();

    try {
      // The entity shows in its kind's list and opens its page.
      await page.goto('/entities');
      await expect(
        page.getByRole('heading', { level: 1, name: 'Organisations, places & pets' }),
      ).toBeVisible();
      await page.getByLabel('Search by name').fill(`Acme ${suffix}`);
      await page.getByRole('link', { name: new RegExp(name) }).click();
      await expect(page.getByRole('heading', { level: 1, name })).toBeVisible();

      // Link the contact to it, as the primary organisation, from the contact page.
      await page.goto(`/contacts/${contact.ID}`);
      const section = page.locator('#entities');
      await section.getByRole('button', { name: 'Link', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Link to an organisation, place or pet' });
      await dialog.getByLabel('Organisation, place or pet').fill(`Acme ${suffix}`);
      await page.getByRole('option', { name }).click();
      await dialog.getByRole('checkbox', { name: /Primary/ }).check();
      await dialog.getByRole('button', { name: 'Save' }).click();
      await expect(section.getByRole('link', { name })).toBeVisible();
      await expect(section.getByText('Shown as organisation')).toBeVisible();

      // The entity page now lists the contact as a primary person.
      await page.goto(`/entities/${entity.id}`);
      const people = page.getByRole('region', { name: 'People' });
      await expect(people.getByRole('link', { name: new RegExp(`E2EEnt${suffix}`) })).toBeVisible();
      await expect(people.getByText('Primary')).toBeVisible();

      // Deleting says who loses the organisation, and can be undone.
      await page.getByRole('button', { name: 'Delete', exact: true }).click();
      const confirm = page.getByRole('dialog', { name: `Delete ${name}?` });
      await expect(confirm.getByText(/is the primary organisation of 1 contact/)).toBeVisible();
      await confirm.getByRole('button', { name: 'Delete', exact: true }).click();
      await expect(page.getByText(`${name} was deleted`)).toBeVisible();
      await page.getByRole('button', { name: 'Restore' }).click();
      await expect(page.getByRole('heading', { level: 1, name })).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
      // Deliberately no DELETE /entities/:id here: the entity is live again after
      // the Restore above, and an entity delete is a soft delete with no purge
      // endpoint, so it would leave a trashed entity in this worker user's Trash.
      // trash.spec.ts's "Empty trash" (contacts only) runs as the same per-worker
      // user and would then never reach the "Trash is empty" state (issue #2484).
      // The worker user's account is deleted at worker teardown, which removes it.
    }
  });
});
