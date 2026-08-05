import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Life events (T5 / WP-84) — a pre-alpha ticket with no e2e coverage.
 *
 * Worth exercising end to end because LifeEvent is one of the entities that
 * keys off Contact.VCardUID rather than the numeric contact ID, and it carries
 * a partial date (year-only / month-day / full) that has to survive the round
 * trip through the API and back onto the contact's tab.
 */
test.describe('Life events', () => {
  test('creates a life event and shows it on the contact', async ({ page, request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}LifeEvent`,
      lastname: 'Subject',
    });

    try {
      const create = await request.post(`${API_BASE_URL}/life-events`, {
        data: {
          entity_id: contact.uid,
          type: 'graduated',
          description: 'Finished their doctorate',
          date: { year: 2021, month: 6, day: 15 },
        },
      });
      expect(create.ok(), `create failed: ${await create.text()}`).toBeTruthy();

      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      // Rendered twice (list row + its title attribute), hence .first().
      await expect(page.getByText('Finished their doctorate').first()).toBeVisible({ timeout: 15000 });
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('supports a year-only partial date', async ({ request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}PartialDate`,
      lastname: 'Subject',
    });

    try {
      // "Some time in 2015" is a real thing users know and the model
      // deliberately supports; month/day stay unset rather than defaulting.
      const create = await request.post(`${API_BASE_URL}/life-events`, {
        data: {
          entity_id: contact.uid,
          type: 'moved',
          description: 'Moved to Lisbon',
          date: { year: 2015 },
        },
      });
      expect(create.ok(), `create failed: ${await create.text()}`).toBeTruthy();

      const list = await request.get(`${API_BASE_URL}/life-events?entity_id=${contact.uid}&limit=50`);
      expect(list.ok()).toBeTruthy();
      const body = await list.json();
      const event = (body.life_events ?? []).find(
        (e: { description: string }) => e.description === 'Moved to Lisbon'
      );
      expect(event, 'the life event must round-trip through the list endpoint').toBeTruthy();
      expect(event.date?.year).toBe(2015);
      expect(event.date?.month, 'a year-only date must not invent a month').toBeUndefined();
      expect(event.date?.day, 'a year-only date must not invent a day').toBeUndefined();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('deletes a life event', async ({ request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}LifeEventDel`,
      lastname: 'Subject',
    });

    try {
      const create = await request.post(`${API_BASE_URL}/life-events`, {
        data: { entity_id: contact.uid, type: 'new_job', description: 'Started at Acme', date: { year: 2024 } },
      });
      expect(create.ok()).toBeTruthy();
      const created = await create.json();
      const eventId = created.life_event?.id ?? created.id;

      const del = await request.delete(`${API_BASE_URL}/life-events/${eventId}`);
      expect(del.ok(), `delete failed: ${await del.text()}`).toBeTruthy();

      const list = await request.get(`${API_BASE_URL}/life-events?entity_id=${contact.uid}&limit=50`);
      const body = await list.json();
      expect(body.life_events ?? []).toHaveLength(0);
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('life events are scoped to their owner', async ({ request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}LifeEventScope`,
      lastname: 'Subject',
    });

    try {
      const create = await request.post(`${API_BASE_URL}/life-events`, {
        data: { entity_id: contact.uid, type: 'married', description: 'Got married', date: { year: 2020 } },
      });
      expect(create.ok()).toBeTruthy();

      // A life event keyed to a VCardUID that is not this user's must not be
      // creatable — the graph entities scope by Contact.VCardUID ownership.
      const foreign = await request.post(`${API_BASE_URL}/life-events`, {
        data: {
          entity_id: '00000000-0000-4000-8000-000000000000',
          type: 'married',
          description: 'Should not exist',
          date: { year: 2020 },
        },
      });
      expect(foreign.ok(), 'a life event on an unknown contact must be rejected').toBeFalsy();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  // Regression coverage for a bug T36's review found (not introduced by it,
  // but sitting silently in the exact save path T36 rewired): ContactDetail
  // Page's LifeEventDialog `onSave` handler blind-spread the dialog's
  // camelCase LifeEventFormData into the API payload, so `relatedEntityIds`
  // never became `related_entity_ids` and picked related contacts were
  // silently dropped on every save. TS couldn't catch it (spreads skip
  // excess-property checks), and no test exercised the UI save path with a
  // related contact selected — only this UI-driven save can catch it; a
  // direct API POST bypasses the dialog entirely.
  test('a related contact picked in the dialog survives the save', async ({ page, request }) => {
    const subject = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}RelatedSubject`,
      lastname: 'Subject',
    });
    const related = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}RelatedOther`,
      lastname: 'Other',
    });

    try {
      await page.goto(`/contacts/${subject.ID}`);
      await waitForLoading(page);

      await page.getByRole('button', { name: /add life event/i }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible({ timeout: 10000 });

      await dialog.getByLabel('Category *').click();
      await page.getByRole('option', { name: 'Family & Relationships', exact: true }).click();
      await dialog.getByLabel('Event Type *').click();
      await page.getByRole('option', { name: 'Started a relationship', exact: true }).click();

      const autocomplete = dialog.getByRole('combobox', { name: /related to/i });
      await autocomplete.fill(related.lastname);
      await page.getByRole('option', { name: new RegExp(related.lastname) }).click();

      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden({ timeout: 10000 });

      // Confirm through the API, not just the UI chip -- proves the value
      // actually round-tripped through the backend, not just local state.
      const list = await request.get(`${API_BASE_URL}/life-events?entity_id=${subject.uid}&limit=10`);
      expect(list.ok()).toBeTruthy();
      const body = await list.json();
      const event = (body.life_events ?? []).find(
        (e: { type: string }) => e.type === 'started_a_relationship'
      );
      expect(event, 'the life event must have been created').toBeTruthy();
      expect(event.related_entity_ids, 'the picked related contact must have been saved').toEqual([related.uid]);
    } finally {
      await deleteTestContact(request, subject.ID);
      await deleteTestContact(request, related.ID);
    }
  });

  // T36 (docs/fork-plan/tickets/45-T36-life-event-categories.md): the
  // cascading category -> type picker, its per-category custom-type
  // affordance, and the "Other / Uncategorized" bucket for a pre-existing
  // event with no category.
  test.describe('T36 category picker', () => {
    test('creates a life event via the cascading category -> type picker', async ({ page, request }) => {
      const contact = await createTestContact(request, {
        firstname: `${E2E_CONTACT_PREFIX}CategoryPicker`,
        lastname: 'Subject',
      });

      try {
        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        await page.getByRole('button', { name: /add life event/i }).click();
        const dialog = page.getByRole('dialog');
        await expect(dialog).toBeVisible({ timeout: 10000 });

        await dialog.getByLabel('Category *').click();
        await page.getByRole('option', { name: 'Home & Living', exact: true }).click();

        await dialog.getByLabel('Event Type *').click();
        await page.getByRole('option', { name: 'Bought a home', exact: true }).click();

        await dialog.getByRole('button', { name: /^save$/i }).click();
        await expect(dialog).toBeHidden({ timeout: 10000 });

        await expect(page.getByText('Bought a home')).toBeVisible();
        await expect(page.getByText('Home & Living')).toBeVisible();
      } finally {
        await deleteTestContact(request, contact.ID);
      }
    });

    test('creates a custom life event type via "Add a new life event type"', async ({ page, request }) => {
      const contact = await createTestContact(request, {
        firstname: `${E2E_CONTACT_PREFIX}CustomType`,
        lastname: 'Subject',
      });

      try {
        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);

        await page.getByRole('button', { name: /add life event/i }).click();
        const dialog = page.getByRole('dialog');
        await expect(dialog).toBeVisible({ timeout: 10000 });

        await dialog.getByLabel('Category *').click();
        await page.getByRole('option', { name: 'Health & Wellness', exact: true }).click();

        await dialog.getByLabel('Event Type *').click();
        await page.getByRole('option', { name: 'Add a new life event type', exact: true }).click();

        await dialog.getByLabel('Custom event name *').fill('Ran a marathon');
        await dialog.getByRole('button', { name: /^save$/i }).click();
        await expect(dialog).toBeHidden({ timeout: 10000 });

        await expect(page.getByText('Ran a marathon')).toBeVisible();
        await expect(page.getByText('Health & Wellness')).toBeVisible();
      } finally {
        await deleteTestContact(request, contact.ID);
      }
    });

    test('editing re-files an event under a different category, pre-filling the existing one first', async ({ page, request }) => {
      const contact = await createTestContact(request, {
        firstname: `${E2E_CONTACT_PREFIX}RecategorizeEdit`,
        lastname: 'Subject',
      });

      try {
        const create = await request.post(`${API_BASE_URL}/life-events`, {
          data: { entity_id: contact.uid, type: 'married', category: 'family_relationships', date: { year: 2020 } },
        });
        expect(create.ok(), `create failed: ${await create.text()}`).toBeTruthy();

        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);
        await expect(page.getByText('Married').first()).toBeVisible();

        // LifeEventList nests the type label one Box deeper than
        // RelationshipEdgeList does (an extra icon+type+chips row Box), so
        // this needs three '..' to reach the Paper's outer flex Box -- the
        // actual common ancestor of the text and the hover-revealed
        // life-event-actions Box -- not two. Two lands on a sibling of the
        // actions Box, so getByLabel('Edit') below would never match
        // anything and the click would time out deterministically.
        const card = page.locator('text=Married').first().locator('..').locator('..').locator('..');
        await card.hover();
        await card.getByLabel('Edit').click();

        const dialog = page.getByRole('dialog');
        await expect(dialog).toBeVisible({ timeout: 10000 });
        // Pre-filled from the existing event before any change is made.
        await expect(dialog.getByLabel('Category *')).toHaveText('Family & Relationships');
        await expect(dialog.getByLabel('Event Type *')).toHaveText('Married');

        await dialog.getByLabel('Category *').click();
        await page.getByRole('option', { name: 'Travel & Experiences', exact: true }).click();
        await dialog.getByLabel('Event Type *').click();
        await page.getByRole('option', { name: 'Traveled', exact: true }).click();

        await dialog.getByRole('button', { name: /^save$/i }).click();
        await expect(dialog).toBeHidden({ timeout: 10000 });

        await expect(page.getByText('Traveled')).toBeVisible();
        await expect(page.getByText('Travel & Experiences')).toBeVisible();
        await expect(page.getByText('Married')).toBeHidden();
      } finally {
        await deleteTestContact(request, contact.ID);
      }
    });

    test('an event with no category (pre-migration/legacy) edits gracefully via the Other / Uncategorized bucket', async ({ page, request }) => {
      const contact = await createTestContact(request, {
        firstname: `${E2E_CONTACT_PREFIX}Uncategorized`,
        lastname: 'Subject',
      });

      try {
        // No category in the payload — mirrors a pre-T36 row the migration
        // left NULL because its type didn't map onto one of the seven
        // original constants.
        const create = await request.post(`${API_BASE_URL}/life-events`, {
          data: { entity_id: contact.uid, type: 'started a podcast' },
        });
        expect(create.ok(), `create failed: ${await create.text()}`).toBeTruthy();

        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);
        await expect(page.getByText('started a podcast').first()).toBeVisible();

        // See the identical note in the "editing re-files an event" test
        // above -- three '..' is required for LifeEventList's DOM depth.
        const card = page.locator('text=started a podcast').first().locator('..').locator('..').locator('..');
        await card.hover();
        await card.getByLabel('Edit').click();

        const dialog = page.getByRole('dialog');
        await expect(dialog).toBeVisible({ timeout: 10000 });
        await expect(dialog.getByLabel('Category *')).toHaveText('Other / Uncategorized');
        // Uncategorized renders Type as a plain free-text field showing the
        // raw stored value, not a picker with nothing selected.
        await expect(dialog.getByLabel('Event Type *')).toHaveValue('started a podcast');

        await dialog.getByRole('button', { name: /^cancel$/i }).click();
        await expect(dialog).toBeHidden({ timeout: 10000 });
      } finally {
        await deleteTestContact(request, contact.ID);
      }
    });
  });
});
