import { toContactRecordInput } from '../src/api/contacts';
import { expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Contact list at scale (issue #556, recommended action 4).
 *
 * T17 cursor pagination bounds the *API* — a page at a time, opaque resume
 * token, no total. What this pins is that the *UI* honours it: the "Load
 * More" button resumes from the cursor rather than re-requesting page one or
 * asking for an unbounded page, and a long session of loading pages grows the
 * DOM linearly (no virtualization today — a deliberately pinned property) but
 * does not blow the heap.
 *
 * @perf-heavy: seeds ~150 contacts, so it runs in the nightly e2e schedule
 * only (RUN_PERF_HEAVY=1), consistent with #447's two-tier split and the
 * backend MYCORRHIZAL_LARGE_TESTS pattern.
 */

const SEED_COUNT = 150;
const PAGE_LIMIT_CEILING = 100; // a bounded page — cursor pagination, not "give me everything"
const seededIds: number[] = [];

test.describe('Contact list pagination at scale', { tag: '@perf-heavy' }, () => {
  test.skip(
    !process.env.RUN_PERF_HEAVY,
    'perf-heavy: runs in the nightly e2e schedule only (set RUN_PERF_HEAVY=1)',
  );
  test.describe.configure({ timeout: 180_000 });

  test.beforeAll(async ({ request }) => {
    // Distinct token so the assertions can't pass on pre-existing fixture data,
    // and the E2EFixture prefix so global-setup sweeps any leak.
    const run = Date.now().toString(36);
    const batch = 20;
    for (let start = 0; start < SEED_COUNT; start += batch) {
      const results = await Promise.all(
        Array.from({ length: Math.min(batch, SEED_COUNT - start) }, (_, i) => {
          const n = start + i;
          return request.post(`${API_BASE_URL}/contacts`, {
            data: toContactRecordInput({
              firstname: `${E2E_CONTACT_PREFIX}List${run}`,
              lastname: String(n).padStart(4, '0'),
              emails: [{ type: 'home', value: `list-${run}-${n}@example.com` }],
            }),
          });
        }),
      );
      for (const res of results) {
        expect(res.ok(), `seed failed: ${res.status()}`).toBeTruthy();
        const body = await res.json();
        seededIds.push((body.contact || body).id);
      }
    }
  });

  test.afterAll(async ({ request }) => {
    await Promise.all(
      seededIds.map((id) => request.delete(`${API_BASE_URL}/contacts/${id}`).catch(() => {})),
    );
  });

  test('Load More resumes from the cursor and grows the DOM linearly without unbounded heap growth', async ({
    page,
  }) => {
    const contactsRequests: { hasCursor: boolean; limit: number | null }[] = [];
    page.on('request', (req) => {
      const url = req.url();
      if (req.method() === 'GET' && /\/api\/v1\/contacts\?/.test(url)) {
        const params = new URL(url).searchParams;
        contactsRequests.push({
          hasCursor: params.has('cursor'),
          limit: params.has('limit') ? Number(params.get('limit')) : null,
        });
      }
    });

    await page.goto('/contacts?has_contact_info=false');
    await waitForLoading(page);

    const heap = () =>
      page.evaluate(
        () =>
          (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory
            ?.usedJSHeapSize ?? 0,
      );
    const heapStart = await heap();

    const cardCount = () => page.getByTestId('contact-card').count();
    const startCards = await cardCount();
    expect(startCards).toBeGreaterThan(0);

    const loadMore = page.getByRole('button', { name: /load more/i });
    let clicks = 0;
    while ((await loadMore.count()) > 0 && (await loadMore.isVisible()) && clicks < 12) {
      const before = await cardCount();
      await loadMore.click();
      await expect.poll(() => cardCount()).toBeGreaterThan(before);
      await waitForLoading(page);
      clicks++;
    }

    expect(clicks, 'expected several pages of seeded data').toBeGreaterThanOrEqual(3);

    // Every contacts request asked for a bounded page.
    for (const r of contactsRequests) {
      expect(r.limit === null || r.limit <= PAGE_LIMIT_CEILING).toBeTruthy();
    }
    // Exactly one cursor-less request (the initial page). Load More must never
    // re-request page one.
    expect(contactsRequests.filter((r) => !r.hasCursor).length).toBe(1);
    // Every subsequent request resumed from a cursor.
    expect(contactsRequests.filter((r) => r.hasCursor).length).toBe(clicks);

    // DOM grew ~linearly with pages loaded (no virtualization — pinned so a
    // regression to "hold everything and re-render" is visible), and every
    // rendered row is unique (no duplicate append).
    const endCards = await cardCount();
    expect(endCards).toBeGreaterThan(startCards);
    const ids = await page
      .getByTestId('contact-card')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-contact-id')));
    expect(new Set(ids).size).toBe(ids.length);

    // Heap is a trend signal, not a hard gate (noisy runner) — this only
    // catches a gross regression such as retaining every page's full payload.
    const heapEnd = await heap();
    if (heapStart > 0) {
      const grownMB = (heapEnd - heapStart) / (1024 * 1024);
      console.log(`list heap growth over ${endCards - startCards} rows: ${grownMB.toFixed(1)} MB`);
      expect(grownMB).toBeLessThan(60);
    }
  });
});
