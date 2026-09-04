import { expect, test, waitForLoading } from './fixtures';

/**
 * Search-as-you-type cost (issue #556, recommended action 5).
 *
 * The backend FTS cost is #469's; the perceived cost is a client concern:
 * whether each keystroke fires a request, and whether a stale response can
 * overwrite a newer one. The last-write-wins guard is unit-tested in
 * useContacts.test.ts; this pins the request *volume* end to end against the
 * real backend — the 300 ms debounce and the two-character minimum in
 * ContactsPage.tsx must actually hold.
 */
test.describe('Contacts search request volume', { tag: '@perf' }, () => {
  test('debounces keystrokes into a single search request and sends none for a one-character query', async ({
    page,
  }) => {
    const searchRequests: string[] = [];
    page.on('request', (req) => {
      const url = req.url();
      if (req.method() === 'GET' && /\/api\/v1\/contacts\?/.test(url) && /[?&]search=/.test(url)) {
        searchRequests.push(new URL(url).searchParams.get('search') ?? '');
      }
    });

    await page.goto('/contacts');
    await waitForLoading(page);

    const field = page.getByLabel('Search contacts…');

    // One character: under the two-rune gate, so no search request at all.
    await field.pressSequentially('a', { delay: 40 });
    await page.waitForTimeout(600);
    expect(searchRequests, 'a 1-char query must not hit the API').toEqual([]);

    // Type the rest quickly. The 300 ms debounce should collapse the burst to
    // a single trailing request for the final value.
    await field.pressSequentially('lice', { delay: 40 });
    await page.waitForTimeout(800);

    expect(
      searchRequests.length,
      `expected 1 debounced request, got ${searchRequests.length}`,
    ).toBe(1);
    expect(searchRequests[0]).toBe('alice');
  });
});
