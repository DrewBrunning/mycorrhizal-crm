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
      await page.getByRole('tab', { name: /life events/i }).click();

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
});
