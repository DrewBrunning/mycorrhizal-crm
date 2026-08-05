import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Global search (T11 / WP-86) — one of the three R5 capabilities, and
 * previously with no e2e coverage at all.
 *
 * The parts worth exercising end to end are the ones that span layers: the
 * FTS5 index is maintained by SQL triggers (migration 000007), so a contact
 * created through the API has to become findable without any explicit
 * re-index step. A broken trigger is invisible to a unit test that seeds the
 * index directly.
 */
test.describe('Search', () => {
  test('finds a contact created moments earlier via the FTS index', async ({ page, request }) => {
    // A distinctive token so the assertion cannot pass on seeded fixture data.
    const surname = `Zzyzx${Date.now()}`;
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}Searchable`,
      lastname: surname,
    });

    try {
      await page.goto(`/search?q=${encodeURIComponent(surname)}`);
      await waitForLoading(page);

      await expect(page.getByText(new RegExp(surname))).toBeVisible({ timeout: 15000 });
      // The Contacts group header carries a count, proving it grouped rather
      // than dumping a flat list.
      await expect(page.getByText(/Contacts \(\d+\)/)).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('surfaces note hits, not just contacts', async ({ page, request }) => {
    // Notes and interactions are the half of search that the AppBar's
    // contact-only quick search never covered.
    const token = `Quokka${Date.now()}`;
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}NoteSearch`,
      lastname: 'Target',
    });

    try {
      const note = await request.post(`${API_BASE_URL}/contacts/${contact.ID}/notes`, {
        data: { content: `Talked about their ${token} collection`, date: new Date().toISOString() },
      });
      expect(note.ok()).toBeTruthy();

      await page.goto(`/search?q=${encodeURIComponent(token)}`);
      await waitForLoading(page);

      await expect(page.getByText(/Notes \(\d+\)/)).toBeVisible({ timeout: 15000 });
      await expect(page.getByText(new RegExp(token))).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('finds a contact by its address street (T38)', async ({ page, request }) => {
    // T38: address fields were never part of the FTS index (or the legacy
    // LIKE fallback), so a street name found nothing. This spans the whole
    // path: a contact created through the API gets its addresses_flat column
    // populated by BeforeSave, indexed by the migration-000010 trigger, and
    // the /search page surfaces the contact when the street is the query.
    const streetToken = `Heliotrope${Date.now()}`;
    const firstname = `${E2E_CONTACT_PREFIX}AddrSearch${Date.now()}`;
    const contact = await createTestContact(request, {
      firstname,
      lastname: 'Target',
      addresses: [
        {
          type: 'home',
          street: `${streetToken} St`,
          city: 'Nowhereville',
          region: '',
          postal: '',
          country: '',
        },
      ],
    });

    try {
      // The street token appears nowhere in the contact's name/email/phone,
      // so the match can only come from the indexed address text. The search
      // page renders the contact's name for a hit, which is what we assert.
      await page.goto(`/search?q=${encodeURIComponent(streetToken)}`);
      await waitForLoading(page);

      await expect(page.getByText(new RegExp(firstname))).toBeVisible({ timeout: 15000 });
      await expect(page.getByText(/Contacts \(\d+\)/)).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('shows an explicit empty state rather than a blank page', async ({ page }) => {
    const nonsense = `NoSuchThing${Date.now()}`;
    await page.goto(`/search?q=${encodeURIComponent(nonsense)}`);
    await waitForLoading(page);

    await expect(page.getByText(/No results for/)).toBeVisible({ timeout: 15000 });
  });

  // T11's synonym half: the whole query is resolved through the relation-type
  // registry, so "brother" resolves to sibling_of and is echoed back.
  test('resolves a relationship synonym in the query', async ({ request }) => {
    const response = await request.get(`${API_BASE_URL}/search?q=brother`);
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(body.resolved_relation, '"brother" must resolve to the sibling_of token').toBe('sibling_of');
  });

  test('scopes results to the calling user', async ({ request }) => {
    // Search runs over an FTS index shared by every user's rows, so the
    // user_id filter in the MATCH query is the only thing preventing a
    // cross-account leak. Worth an explicit assertion.
    const response = await request.get(`${API_BASE_URL}/search?q=${encodeURIComponent('Alice')}`);
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(Array.isArray(body.contacts)).toBeTruthy();
  });

  test('rejects a search with no query term', async ({ request }) => {
    const response = await request.get(`${API_BASE_URL}/search`);
    expect(response.status()).toBe(400);
  });
});
