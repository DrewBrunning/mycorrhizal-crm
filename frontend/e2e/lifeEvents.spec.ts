import { test, expect, Page } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading, stableClick } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Finds a specific event's card within the Life Events panel specifically --
 * not just `page.locator('text=...')`, which is ambiguous here.
 * ContactTimeline renders the same life event a second time in its own
 * combined feed (a separate PanelCard titled "Timeline", above this one),
 * so `text=Married` alone matches multiple elements and `.first()` isn't
 * guaranteed to land on the Life Events entry -- confirmed directly: for a
 * "married" event it consistently picked the ContactTimeline occurrence
 * instead, which renders no Edit button at all for life-event items
 * (ContactTimeline only gives note/activity items one), so the derived card
 * had zero matches and every downstream wait/click hung until the test's
 * own timeout. Scoping to the "Life Events" PanelCard's CardContent first
 * makes this unambiguous regardless of what ContactTimeline also renders.
 */
function lifeEventCard(page: Page, typeText: string) {
  const panel = page.getByRole('heading', { name: 'Life Events', exact: true }).locator('..').locator('..');
  return panel.locator('.MuiPaper-root').filter({ hasText: typeText });
}

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
    // Lastname carries a timestamp suffix (matching relationshipEdges.spec.ts's
    // own convention) rather than a fixed 'Other' -- a fixed name is only
    // unique within a single clean CI run; under any repeated/retried
    // execution against a shared database (e.g. local --repeat-each stress
    // runs, or two retries whose cleanup races), a second contact can
    // transiently coexist with the same name and turn the option lookup below
    // into a strict-mode violation (multiple matching options). Confirmed via
    // a --repeat-each=15 local run.
    const related = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}RelatedOther`,
      lastname: `Other${Date.now()}`,
    });

    try {
      await page.goto(`/contacts/${subject.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: /add life event/i }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible({ timeout: 10000 });

      await stableClick(dialog.getByLabel('Category *'));
      await stableClick(page.getByRole('option', { name: 'Family & Relationships', exact: true }));
      await stableClick(dialog.getByLabel('Event Type *'));
      await stableClick(page.getByRole('option', { name: 'Started a relationship', exact: true }));

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

  // T36: the
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

        await stableClick(page.getByRole('button', { name: /add life event/i }));
        const dialog = page.getByRole('dialog');
        await expect(dialog).toBeVisible({ timeout: 10000 });

        await stableClick(dialog.getByLabel('Category *'));
        await stableClick(page.getByRole('option', { name: 'Home & Living', exact: true }));

        await stableClick(dialog.getByLabel('Event Type *'));
        await stableClick(page.getByRole('option', { name: 'Bought a home', exact: true }));

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

        await stableClick(page.getByRole('button', { name: /add life event/i }));
        const dialog = page.getByRole('dialog');
        await expect(dialog).toBeVisible({ timeout: 10000 });

        await stableClick(dialog.getByLabel('Category *'));
        await stableClick(page.getByRole('option', { name: 'Health & Wellness', exact: true }));

        await stableClick(dialog.getByLabel('Event Type *'));
        await stableClick(page.getByRole('option', { name: 'Add a new life event type', exact: true }));

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

        // See lifeEventCard's own doc comment -- 'text=Married' alone is
        // ambiguous (ContactTimeline renders the same event a second time,
        // with no Edit button on that copy) and consistently picked the
        // wrong occurrence for this exact event type.
        const card = lifeEventCard(page, 'Married');
        await card.hover();
        await stableClick(card.getByLabel('Edit'));

        const dialog = page.getByRole('dialog');
        await expect(dialog).toBeVisible({ timeout: 10000 });
        // Pre-filled from the existing event before any change is made.
        await expect(dialog.getByLabel('Category *')).toHaveText('Family & Relationships');
        await expect(dialog.getByLabel('Event Type *')).toHaveText('Married');

        // stableClick, not a plain .click(): the dialog's content is already
        // correct by this point (confirmed above), but MUI's Dialog Grow
        // transition can still be animating the dialog's position/scale for
        // a beat after that -- a plain click here raced it and opened the
        // dropdown at a stale position, so the *next* click (selecting the
        // option) landed back on the still-open previous dropdown instead.
        await stableClick(dialog.getByLabel('Category *'));
        await stableClick(page.getByRole('option', { name: 'Travel & Experiences', exact: true }));
        await stableClick(dialog.getByLabel('Event Type *'));
        await stableClick(page.getByRole('option', { name: 'Traveled', exact: true }));

        await dialog.getByRole('button', { name: /^save$/i }).click();
        await expect(dialog).toBeHidden({ timeout: 10000 });

        // .first(): ContactTimeline renders the event's type a second time
        // in its own combined feed (see lifeEventCard's doc comment) --
        // "Traveled" legitimately appears twice now, same as "Married" did
        // before the edit.
        await expect(page.getByText('Traveled').first()).toBeVisible();
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
        // waitForLoading only tracks the page-level Skeleton/progressbar,
        // which clears once `record` itself has loaded -- it cannot see
        // useLifeEvents' own internal `loading` state, since LifeEventList
        // renders no indicator while its own fetch (triggered by
        // record.uid becoming available, so it starts *after* the
        // page-level Skeleton is already gone) is still in flight. Give
        // this first content assertion room for that extra round trip
        // rather than the default 5s, matching the 10s used for dialog
        // visibility elsewhere in this file. Confirmed via a one-off
        // failure under heavy concurrent local load.
        await expect(page.getByText('started a podcast').first()).toBeVisible({ timeout: 10000 });

        // See lifeEventCard's own doc comment for why this must be scoped
        // to the Life Events panel rather than a bare text= locator.
        const card = lifeEventCard(page, 'started a podcast');
        await card.hover();
        await stableClick(card.getByLabel('Edit'));

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
