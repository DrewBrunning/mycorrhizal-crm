import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact } from './fixtures';
import { API_BASE_URL } from './global-setup';

/**
 * T73 — the
 * contacts list endpoint now supports ?sort=name, ordering by a denormalized
 * sort_name key (lower(trim(lastname)) else lower(trim(firstname))). This
 * ticket is backend-only: the web sort control that consumes it is T77, so
 * the e2e surface here is the API itself, driven through Playwright's
 * authenticated `request` fixture exactly like the T69/T38 search specs.
 *
 * The parts worth exercising end to end are the ones that span layers: a
 * contact created through the API must be findable and correctly ordered by
 * its sort_name WITHOUT any explicit re-derivation step (Contact.BeforeSave
 * populates the column on create). A regression in the BeforeSave wiring, or
 * in the migration's backfill for pre-existing rows, is invisible to a unit
 * test that seeds sort_name directly.
 */
test.describe('Contacts list name sort (T73)', () => {
  test('orders contacts by name in both directions', async ({ request }) => {
    const ts = Date.now();
    const created = await Promise.all([
      createTestContact(request, { firstname: `E2EFixtureT73SortZ${ts}`, lastname: 'T73Zulu' }),
      createTestContact(request, { firstname: `E2EFixtureT73SortA${ts}`, lastname: 'T73Alpha' }),
      createTestContact(request, { firstname: `E2EFixtureT73SortM${ts}`, lastname: 'T73Mike' }),
      createTestContact(request, { firstname: `E2EFixtureT73SortOnly${ts}` }), // no lastname
    ]);

    try {
      // search= isolates this spec's contacts so the assertions can't be
      // pushed off the page by the many other contacts a full parallel run
      // creates at the same time.
      const asc = await fetchNameSorted(request, 'asc', 'E2EFixtureT73Sort');
      const byKey = (c: { firstname: string; lastname?: string }) =>
        (c.lastname || '').trim().toLowerCase() || c.firstname.trim().toLowerCase();

      expect(asc.map((c) => byKey(c))).toEqual(
        created.map((c) => byKey(c)).sort(),
        'ascending name sort must order by lower(trim(lastname)) else lower(trim(firstname))'
      );

      const desc = await fetchNameSorted(request, 'desc', 'E2EFixtureT73Sort');
      expect(desc.map((c) => byKey(c))).toEqual(
        created.map((c) => byKey(c)).sort().reverse(),
        'descending name sort must reverse the ascending order'
      );
    } finally {
      for (const c of created) await deleteTestContact(request, c.ID);
    }
  });

  test('pages through a name-sorted list returning every contact exactly once', async ({ request }) => {
    const ts = Date.now();
    // Two contacts share a lastname (a sort_name tie — the case the id
    // tiebreak exists for); one has no lastname at all. limit=3 makes the
    // tied pair straddle a page boundary (one ends page 1, the other starts
    // page 2), so the id tiebreak is what keeps them ordered across pages.
    const created = await Promise.all([
      createTestContact(request, { firstname: `E2EFixtureT73PageA${ts}`, lastname: 'T73Shared' }),
      createTestContact(request, { firstname: `E2EFixtureT73PageB${ts}`, lastname: 'T73Alpha' }),
      createTestContact(request, { firstname: `E2EFixtureT73PageC${ts}`, lastname: 'T73Shared' }),
      createTestContact(request, { firstname: `E2EFixtureT73PageD${ts}` }),
    ]);

    try {
      const walked = await walkNameSortedPages(request, 'asc', 3, 'E2EFixtureT73Page');

      expect(walked.length).toBe(4);
      // Exactly once each.
      const ids = walked.map((c) => c.id);
      expect(new Set(ids).size).toBe(4);

      // In (sort_key, id) order — the id tiebreak inside the Shared run.
      const byKey = (c: { firstname: string; lastname?: string }) =>
        (c.lastname || '').trim().toLowerCase() || c.firstname.trim().toLowerCase();
      const expected = [...walked].sort((a, b) => {
        const ka = byKey(a);
        const kb = byKey(b);
        if (ka < kb) return -1;
        if (ka > kb) return 1;
        return a.id - b.id;
      });
      expect(walked.map((c) => c.id)).toEqual(expected.map((c) => c.id));
    } finally {
      for (const c of created) await deleteTestContact(request, c.ID);
    }
  });

  test('rejects an unknown sort value with 400', async ({ request }) => {
    const response = await request.get(`${API_BASE_URL}/contacts?sort=firstname`);
    expect(response.status()).toBe(400);
  });

  test('rejects sort=name combined with since with 400', async ({ request }) => {
    // The ?since= change feed is sync state, always ordered by (updated_at,
    // id) — never name order. sort=name combined with a since cursor must
    // fail loudly (400), not silently fall back to a feed a sync client
    // could mistake for name-ordered.
    const contact = await createTestContact(request, {
      firstname: `E2EFixtureT73Since${Date.now()}`,
      lastname: 'T73Since',
    });
    try {
      // A real time-based feed cursor, minted by the default-sorted list.
      const firstPage = await request.get(`${API_BASE_URL}/contacts?order=asc&limit=1`);
      expect(firstPage.ok()).toBeTruthy();
      const body = await firstPage.json();
      expect(body.next_cursor).toBeTruthy();

      // sort=name + since is a 400.
      const since = await request.get(`${API_BASE_URL}/contacts?sort=name&since=${body.next_cursor}`);
      expect(since.status()).toBe(400);

      // The same since cursor with the default sort stays a normal feed.
      const feed = await request.get(`${API_BASE_URL}/contacts?sort=updated_at&since=${body.next_cursor}`);
      expect(feed.ok()).toBeTruthy();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });
});

async function fetchNameSorted(
  request: import('@playwright/test').APIRequestContext,
  order: 'asc' | 'desc',
  search: string
): Promise<Array<{ id: number; firstname: string; lastname?: string }>> {
  const response = await request.get(
    `${API_BASE_URL}/contacts?sort=name&order=${order}&limit=100&search=${encodeURIComponent(search)}`
  );
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  return body.contacts;
}

async function walkNameSortedPages(
  request: import('@playwright/test').APIRequestContext,
  order: 'asc' | 'desc',
  limit: number,
  search: string
): Promise<Array<{ id: number; firstname: string; lastname?: string }>> {
  const all: Array<{ id: number; firstname: string; lastname?: string }> = [];
  let cursor = '';
  for (let page = 0; page < 20; page++) {
    const query = `sort=name&order=${order}&limit=${limit}&search=${encodeURIComponent(search)}${cursor ? `&cursor=${cursor}` : ''}`;
    const response = await request.get(`${API_BASE_URL}/contacts?${query}`);
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    all.push(...body.contacts);
    if (!body.next_cursor) break;
    cursor = body.next_cursor;
  }
  return all;
}
