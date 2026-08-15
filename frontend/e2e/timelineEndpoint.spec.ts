import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

/**
 * T66 — backend-only: the paginated, filterable merged timeline
 * endpoint and the bounded M4 contact-detail composite. The web timeline
 * explorer that consumes them is T78, so the e2e surface here is the API
 * itself, driven through Playwright's authenticated `request` fixture exactly
 * like the T73/T69/T38 specs.
 *
 * The parts worth exercising end to end are the ones that span layers: a
 * contact, its notes and external activities created through the real REST
 * API must be merged, ordered, filtered and paged correctly by the new
 * endpoint — a regression in the per-table predicates or the merge is
 * invisible to a Go test that seeds rows directly. And the composite bound
 * must hold for data created through the real pipeline.
 */

async function createNote(
  request: import('@playwright/test').APIRequestContext,
  contactId: number,
  content: string,
  date: string
): Promise<void> {
  const response = await request.post(`${API_BASE_URL}/contacts/${contactId}/notes`, {
    data: { content, date },
  });
  expect(response.ok(), `failed to create note: ${response.status()} ${await response.text()}`).toBeTruthy();
}

async function createExternalActivity(
  request: import('@playwright/test').APIRequestContext,
  entityId: string,
  extId: string,
  occurredAt: string
): Promise<void> {
  const response = await request.post(`${API_BASE_URL}/external-activities`, {
    data: {
      entity_id: entityId,
      source_system: 't66-e2e',
      external_id: extId,
      type: 'photo-appearance',
      occurred_at: occurredAt,
    },
  });
  expect(response.ok(), `failed to create external activity: ${response.status()} ${await response.text()}`).toBeTruthy();
}

interface TimelineItem {
  type: string;
  id: string;
  date: string;
}

test.describe('Contact timeline endpoint (T66)', () => {
  test('merges notes and external activities in date order and pages every item exactly once', async ({ request }) => {
    const ts = Date.now();
    const contact = await createTestContact(request, { firstname: `E2EFixtureT66Merge${ts}` });
    const base = new Date();
    const noteTimes = [
      new Date(base.getTime() - 3 * 60 * 1000).toISOString(),
      new Date(base.getTime() - 2 * 60 * 1000).toISOString(),
      new Date(base.getTime() - 60 * 1000).toISOString(),
    ];
    const extTimes = [
      new Date(base.getTime() - 4 * 60 * 1000).toISOString(),
      new Date(base.getTime() - 5 * 60 * 1000).toISOString(),
    ];

    try {
      for (const [i, d] of noteTimes.entries()) {
        await createNote(request, contact.ID, `T66 note ${i}`, d);
      }
      for (const [i, d] of extTimes.entries()) {
        await createExternalActivity(request, contact.uid, `t66-ext-${ts}-${i}`, d);
      }

      // Walk the whole timeline with a small limit so paging is exercised.
      const walked: Array<{ type: string; id: string; date: string }> = [];
      let cursor = '';
      for (let page = 0; page < 10; page++) {
        const query = `limit=2${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`;
        const response = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/timeline?${query}`);
        expect(response.ok(), await response.text()).toBeTruthy();
        const body = await response.json();
        walked.push(...(body.items as TimelineItem[]));
        if (!body.next_cursor) break;
        cursor = body.next_cursor;
      }

      expect(walked.length).toBe(5);
      // Exactly once each.
      const keys = walked.map((it) => `${it.type}|${it.id}`);
      expect(new Set(keys).size).toBe(5);

      // Descending by date, with the three notes on top (newest first).
      const dates = walked.map((it) => new Date(it.date).getTime());
      expect([...dates].sort((a, b) => b - a)).toEqual(dates);
      expect(walked.slice(0, 3).every((it) => it.type === 'note')).toBeTruthy();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('filters by type and recency bucket', async ({ request }) => {
    const ts = Date.now();
    const contact = await createTestContact(request, { firstname: `E2EFixtureT66Filter${ts}` });
    const now = new Date();
    const recent = new Date(now.getTime() - 60 * 1000).toISOString();
    const old = new Date(now.getTime() - 20 * 24 * 60 * 60 * 1000).toISOString();

    try {
      await createNote(request, contact.ID, 'recent note', recent);
      await createNote(request, contact.ID, 'old note', old);
      await createExternalActivity(request, contact.uid, `t66-filter-ext-${ts}`, recent);

      const typed = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/timeline?type=note`);
      expect(typed.ok()).toBeTruthy();
      const typedBody = await typed.json();
      expect(typedBody.items.length).toBe(2);
      expect(typedBody.items.every((it: TimelineItem) => it.type === 'note')).toBeTruthy();

      const recentBucket = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/timeline?bucket=last_7_days`);
      expect(recentBucket.ok()).toBeTruthy();
      const recentBody = await recentBucket.json();
      expect(recentBody.items.length).toBe(2, 'the 20-day-old note must be excluded by last_7_days');

      const allBucket = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/timeline?bucket=all`);
      const allBody = await allBucket.json();
      expect(allBody.items.length).toBe(3);
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('rejects an unknown type with 400', async ({ request }) => {
    const contact = await createTestContact(request, { firstname: `E2EFixtureT66Bad${Date.now()}` });
    try {
      const response = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/timeline?type=banana`);
      expect(response.status()).toBe(400);
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('bounds the M4 composite timeline blocks for a long-history contact', async ({ request }) => {
    const ts = Date.now();
    const contact = await createTestContact(request, { firstname: `E2EFixtureT66Bound${ts}` });

    try {
      // A dozen external activities + a handful of notes through the real API.
      const now = new Date();
      for (let i = 0; i < 12; i++) {
        await createExternalActivity(
          request,
          contact.uid,
          `t66-bound-ext-${ts}-${i}`,
          new Date(now.getTime() - i * 60 * 1000).toISOString()
        );
      }
      for (let i = 0; i < 6; i++) {
        await createNote(request, contact.ID, `bound note ${i}`, new Date(now.getTime() - i * 60 * 1000).toISOString());
      }

      const response = await request.get(`${API_BASE_URL}/contacts/${contact.ID}/detail`);
      expect(response.ok(), await response.text()).toBeTruthy();
      const body = await response.json();

      for (const block of ['notes', 'external_activities']) {
        expect(Array.isArray(body[block])).toBeTruthy();
        expect(body[block].length).toBeLessThanOrEqual(5, `${block} must be bounded at 5`);
      }
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });
});
