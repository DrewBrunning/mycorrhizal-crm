import { expect, test } from '@playwright/test';
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
  test('the briefing endpoint always returns the collection blocks as arrays', async ({
    request,
  }) => {
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

  test('surfaces a note and reaches the prep view from the contact page', async ({
    page,
    request,
  }) => {
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

  // The all-seven-sections case. Web's prep view renders every block from the
  // briefing composite, and the M11 Android client consumes the exact same
  // wire contract — so this spec is the shared proof that a fully-populated
  // contact yields every section, populated from real backend data (cadence
  // health, agenda, last interaction + notes, relationships, life events,
  // reminders, upcoming dates).
  test('renders every section for a fully populated contact', async ({ page, request }) => {
    // The other party for the relationship edge.
    const other = await createTestContact(request, { firstname: 'Prep', lastname: 'Spouse' });
    const contact = await createTestContact(request, {
      firstname: 'Prep',
      lastname: 'Full',
      // A yearless birthday 7 days out, so the upcoming-dates block is deterministic.
      birthday: (() => {
        const d = new Date();
        d.setDate(d.getDate() + 7);
        const m = String(d.getUTCMonth() + 1).padStart(2, '0');
        const day = String(d.getUTCDate()).padStart(2, '0');
        return `--${m}-${day}`;
      })(),
    });

    try {
      const now = new Date().toISOString();

      // Last interaction — a qualifying activity (visit counts toward cadence).
      const activity = await request.post(`${API_BASE_URL}/activities`, {
        data: { title: 'Coffee', type: 'visit', date: now, contact_ids: [contact.ID] },
      });
      expect(activity.ok(), `activity: ${await activity.text()}`).toBeTruthy();

      const note = await request.post(`${API_BASE_URL}/contacts/${contact.ID}/notes`, {
        data: { content: 'Mentioned their new job', date: now },
      });
      expect(note.ok(), `note: ${await note.text()}`).toBeTruthy();

      const cadence = await request.post(`${API_BASE_URL}/cadence-policies`, {
        data: { entity_id: contact.uid, target_interval_days: 30 },
      });
      expect(cadence.ok(), `cadence: ${await cadence.text()}`).toBeTruthy();

      const agenda = await request.post(`${API_BASE_URL}/conversation-agenda`, {
        data: { entity_id: contact.uid, content: 'Ask about the trip' },
      });
      expect(agenda.ok(), `agenda: ${await agenda.text()}`).toBeTruthy();

      const edge = await request.post(`${API_BASE_URL}/relationship-edges`, {
        data: { source_id: contact.uid, target_id: other.uid, type: 'spouse_of' },
      });
      expect(edge.ok(), `edge: ${await edge.text()}`).toBeTruthy();

      const lifeEvent = await request.post(`${API_BASE_URL}/life-events`, {
        data: { entity_id: contact.uid, type: 'graduated', description: 'MSc' },
      });
      expect(lifeEvent.ok(), `lifeEvent: ${await lifeEvent.text()}`).toBeTruthy();

      const reminder = await request.post(`${API_BASE_URL}/contacts/${contact.ID}/reminders`, {
        data: {
          message: 'Send a card',
          remind_at: new Date(Date.now() + 5 * 24 * 60 * 60 * 1000).toISOString(),
          recurrence: 'once',
          reoccur_from_completion: false,
          contact_id: contact.ID,
        },
      });
      expect(reminder.ok(), `reminder: ${await reminder.text()}`).toBeTruthy();

      await page.goto(`/contacts/${contact.ID}/prep`);
      await waitForLoading(page);

      // Header.
      await expect(page.getByText('Prep Full')).toBeVisible({ timeout: 15000 });

      // Cadence health — the activity just created is qualifying, so the
      // health block is server-derived "on track" with next-due/last dates.
      await expect(page.getByText('Relationship health')).toBeVisible();
      await expect(page.getByText('On track')).toBeVisible();
      await expect(page.getByText(/Next due:/)).toBeVisible();
      await expect(page.getByText(/Last interaction:/)).toBeVisible();

      // Agenda.
      await expect(page.getByText('Things to bring up')).toBeVisible();
      await expect(page.getByText(/Ask about the trip/)).toBeVisible();

      // Last interaction + recent notes.
      await expect(page.getByText('Coffee')).toBeVisible();
      await expect(page.getByText(/Mentioned their new job/)).toBeVisible();

      // Relationships — other party + the label as read from this contact's
      // perspective ("Spouse", not "spouse_of").
      await expect(page.getByText('People around them')).toBeVisible();
      await expect(page.getByText(/Prep Spouse/)).toBeVisible();
      await expect(page.getByText(/Spouse$/)).toBeVisible();

      // Life events.
      await expect(page.getByText('Life events')).toBeVisible();
      await expect(page.getByText(/Graduated/)).toBeVisible();

      // Upcoming reminders.
      await expect(page.getByText('Upcoming reminders')).toBeVisible();
      await expect(page.getByText(/Send a card/)).toBeVisible();

      // Upcoming dates — the birthday set 7 days out.
      await expect(page.getByText('Upcoming dates')).toBeVisible();
      await expect(page.getByText(/in \d+ day/)).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
      await deleteTestContact(request, other.ID);
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
