import { expect, test } from '@playwright/test';
import { createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

/**
 * Issue #257 — live counterpart to src/api/contractFixtures.test.ts. That
 * suite pins parsing against a fixture captured once; this spec hits the
 * running backend directly (API-only, no `page` -- same pattern as
 * contactSort.spec.ts/search.spec.ts's API specs) so a shape regression in
 * *today's* backend fails CI even before anyone re-captures the fixture.
 *
 * Deliberately shallow: this is a shape/status guard, not a content
 * assertion (the fixture suite already owns pinning specific values).
 */
test.describe('API contract: composite endpoints', () => {
  test('GET /contacts/:id/detail returns every collection block as an array', async ({
    request,
  }) => {
    const contact = await createTestContact(request, {
      firstname: 'ApiContract',
      lastname: 'Detail',
    });
    try {
      const response = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/detail`);
      expect(response.ok(), `detail: ${response.status()} ${await response.text()}`).toBeTruthy();

      const body = await response.json();
      for (const key of [
        'notes',
        'activities',
        'completions',
        'reminders',
        'relationship_edges',
        'life_events',
        'agenda',
        'gifts',
        'field_values',
        'external_identities',
        'external_activities',
        'circles',
        'tags',
      ]) {
        expect(Array.isArray(body[key]), `${key} should be an array`).toBe(true);
      }
      expect(body.contact.uid).toBe(contact.uid);
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('GET /dashboard returns every block as an array, never absent', async ({ request }) => {
    const response = await request.get(`${API_BASE_URL}/dashboard`);
    expect(response.ok(), `dashboard: ${response.status()} ${await response.text()}`).toBeTruthy();

    const body = await response.json();
    for (const key of [
      'birthdays',
      'random_contacts',
      'upcoming_reminders',
      'overdue',
      'favorites',
      'reach_out_suggestions',
    ]) {
      expect(Array.isArray(body[key]), `${key} should be an array`).toBe(true);
    }
  });
});
