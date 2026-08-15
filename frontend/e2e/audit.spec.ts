import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';
import { toContactRecordInput } from '../src/api/contacts';

// T60 — the audit log page.
//
// The audit backend (T18) records events fire-and-forget from a goroutine, so
// an event is guaranteed to land shortly AFTER the API call that caused it
// returns, but not necessarily before. Every spec therefore polls the API
// until the event it cares about exists before driving the UI — asserting
// against a not-yet-written row would be testing the goroutine's timing, not
// the page.
test.describe('Audit log', () => {
  test('reachable from navigation, filters by entity, and undoes a contact update end-to-end', async ({ page, request }) => {
    const contact = await createTestContact(request, { lastname: 'Before' });

    try {
      // Update the contact so there is a real before/after to undo.
      const updated = await request.put(`${API_BASE_URL}/contacts/${contact.ID}`, {
        data: toContactRecordInput({ firstname: contact.firstname, lastname: 'After' }),
      });
      expect(updated.ok()).toBeTruthy();

      // Wait for the async audit write to land.
      await expect(async () => {
        const res = await request.get(
          `${API_BASE_URL}/audit?entity_type=contact&entity_id=${contact.uid}&limit=100`
        );
        const body = res.ok() ? await res.json() : { audit_events: [] };
        expect((body.audit_events || []).some((e: { operation: string }) => e.operation === 'update')).toBeTruthy();
      }).toPass({ timeout: 15000 });

      // The page is reachable from app navigation, not a dead URL.
      await page.goto('/');
      await page.getByRole('link', { name: 'Audit log' }).click();
      await expect(page).toHaveURL(/\/audit/);
      await expect(page.getByRole('heading', { name: 'Audit log' })).toBeVisible();

      // Filter by entity ID; both the create and update rows for this contact
      // resolve to its current name, so the update row is the one carrying the
      // "Updated" chip.
      await page.getByLabel('Entity ID').fill(contact.uid);
      const updateRow = page
        .getByRole('row')
        .filter({ hasText: contact.firstname })
        .filter({ has: page.getByText('Updated') });
      await expect(updateRow).toBeVisible({ timeout: 10000 });

      // Undo is offered on the contact-update row.
      const undoButton = updateRow.getByRole('button', { name: 'Undo' });
      await expect(undoButton).toBeVisible();

      // Confirm dialog -> success toast -> list refresh.
      await undoButton.click();
      await expect(page.getByRole('dialog')).toContainText('Undo this change?');
      await page.getByRole('dialog').getByRole('button', { name: 'Undo' }).click();
      await expect(page.getByText('Contact restored to its previous state')).toBeVisible();

      // Hand-verified revert: re-fetch the contact and confirm the field is
      // back to the pre-update value.
      const refetched = await request.get(
        `${API_BASE_URL}/contacts?search=${encodeURIComponent(contact.firstname)}&limit=10`
      );
      expect(refetched.ok()).toBeTruthy();
      const found = (await refetched.json()).contacts.find(
        (c: { id: number }) => c.id === contact.ID
      );
      expect(found.lastname).toBe('Before');
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('Undo is hidden on non-contact update events', async ({ page, request }) => {
    // A circle update is an audit `update` event too, but the undo endpoint
    // only supports contacts (it 400s everything else) — the button must be
    // gated to match, not just hidden after a failed request.
    const circleName = `E2FAuditCircle${Date.now()}`;
    const createRes = await request.post(`${API_BASE_URL}/circles`, { data: { name: circleName } });
    expect(createRes.ok()).toBeTruthy();
    const createdCircle = await createRes.json();
    const circle = createdCircle.circle ?? createdCircle;

    try {
      const updated = await request.put(`${API_BASE_URL}/circles/${circle.id}`, {
        data: { name: `${circleName}-2` },
      });
      expect(updated.ok()).toBeTruthy();

      // Wait for the circle update event to land.
      await expect(async () => {
        const res = await request.get(
          `${API_BASE_URL}/audit?entity_type=circle&entity_id=${circle.id}&limit=100`
        );
        const body = res.ok() ? await res.json() : { audit_events: [] };
        expect((body.audit_events || []).some((e: { operation: string }) => e.operation === 'update')).toBeTruthy();
      }).toPass({ timeout: 15000 });

      await page.goto('/audit');

      // Sanity: the unfiltered list has contact-update rows that DO offer
      // undo, so the following assertions are about the filter, not a page
      // that never renders the button.
      await expect(page.getByRole('button', { name: 'Undo' }).first()).toBeVisible({ timeout: 10000 });

      // Filter to circle events: the circle's row renders (its uuid is the
      // entity cell), but no row offers Undo -- the button is gated on
      // entity_type === contact to match the server's 400 for everything else.
      await page.getByLabel('Entity type').click();
      await page.getByRole('option', { name: 'Circle' }).click();
      await expect(page.getByText(circle.id).first()).toBeVisible({ timeout: 10000 });
      await expect(page.getByRole('button', { name: 'Undo' })).toHaveCount(0, { timeout: 10000 });
    } finally {
      await request.delete(`${API_BASE_URL}/circles/${circle.id}`).catch(() => {});
    }
  });

  test('shows an empty state when no events match the filters', async ({ page }) => {
    await page.goto('/audit');
    await page.getByLabel('Entity ID').fill('no-such-entity-id-12345');
    await expect(
      page.getByText('No audit events match your filters.')
    ).toBeVisible({ timeout: 10000 });
  });
});
