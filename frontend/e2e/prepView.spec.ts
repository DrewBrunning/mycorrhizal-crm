import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

/**
 * Prep view (N2).
 *
 * This spec exists because of a shipped crash: ContactBriefing's six
 * collection blocks were tagged `omitempty` in Go, so a contact with no
 * history serialized without them, and PrepViewPage's
 * `briefing.open_agenda_items.length` threw — taking the whole page into the
 * ErrorBoundary.
 *
 * Every newly-created contact is in exactly that state, so the feature was
 * broken on first use, which is the state an alpha starts in. Neither the Go
 * suite nor the vitest suite could see it: the Go test decoded into a struct
 * where absent and `[]` are both nil, and the vitest fixture passed `[]`, a
 * shape the server could not produce.
 *
 * The e2e layer is the one that would have caught it, because it drives a real
 * API against a real empty database. It had no prep-view spec. It does now.
 */
test.describe('Prep view', () => {
  test('renders for a brand-new contact with no history at all', async ({ page, request }) => {
    // Deliberately bare: no notes, activities, reminders, relationships, life
    // events, agenda items or cadence policy. This is the exact state that
    // crashed, and the state every contact is in the moment it is created.
    const contact = await createTestContact(request, { firstname: 'Prep', lastname: 'Empty' });

    try {
      await page.goto(`/contacts/${contact.ID}/prep`);
      await waitForLoading(page);

      // The contact's own name proves the page rendered rather than the
      // ErrorBoundary's failure surface.
      await expect(page.getByText('Prep Empty')).toBeVisible({ timeout: 15000 });
      await expect(page.getByText('Everything to remember before you see them')).toBeVisible();

      // The empty-state copy, not a crash.
      await expect(page.getByText('No interactions recorded yet')).toBeVisible();

      // Blocks with no data stay hidden rather than rendering empty shells.
      await expect(page.getByText('Things to bring up')).toBeHidden();
      await expect(page.getByText('People around them')).toBeHidden();
      await expect(page.getByText('Relationship health')).toBeHidden();

      // Belt and braces: no React error boundary anywhere on the page.
      await expect(page.getByText(/something went wrong/i)).toBeHidden();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  // Pins the wire contract the crash came from, independently of how the page
  // happens to render it. `omitempty` returning to those fields would fail
  // here even if the frontend's own normalisation masked it.
  test('the briefing endpoint always returns the collection blocks as arrays', async ({ request }) => {
    const contact = await createTestContact(request, { firstname: 'Prep', lastname: 'Wire' });

    try {
      const response = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/briefing`);
      expect(response.ok()).toBeTruthy();
      const briefing = await response.json();

      for (const block of [
        'recent_notes',
        'open_agenda_items',
        'relationships',
        'life_events',
        'upcoming_reminders',
        'upcoming_dates',
      ]) {
        expect(briefing[block], `${block} must be present, not omitted`).toBeDefined();
        expect(Array.isArray(briefing[block]), `${block} must be an array, not null`).toBeTruthy();
      }
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('surfaces a note and reaches the prep view from the contact page', async ({ page, request }) => {
    const contact = await createTestContact(request, { firstname: 'Prep', lastname: 'Populated' });

    try {
      const noteResponse = await request.post(`${API_BASE_URL}/contacts/${contact.ID}/notes`, {
        data: { content: 'Mentioned their new job', date: new Date().toISOString() },
      });
      expect(noteResponse.ok()).toBeTruthy();

      // Navigate the way a user does, rather than deep-linking.
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      await page.getByRole('button', { name: /prep view/i }).click();

      await expect(page).toHaveURL(new RegExp(`/contacts/${contact.ID}/prep`));
      await expect(page.getByText('Recent notes')).toBeVisible({ timeout: 15000 });
      await expect(page.getByText(/Mentioned their new job/)).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('shows a not-found state for a contact that does not exist', async ({ page }) => {
    await page.goto('/contacts/99999999/prep');
    await waitForLoading(page);

    // Two matches by design — the inline Alert and the snackbar — hence
    // .first() rather than a strict single-element assertion.
    //
    // This assertion also pins a bug this spec found: getDisplayMessage used
    // to join ApiError.details' VALUES for every error carrying details, and
    // ErrNotFound attaches {id: "<requested id>"} as context. The user was
    // shown a bare "99999999" instead of "Contact not found" — on every
    // not-found error in the app, not just this page.
    await expect(page.getByText(/contact not found/i).first()).toBeVisible({ timeout: 15000 });
    await expect(page.getByText('99999999')).toBeHidden();
  });
});
