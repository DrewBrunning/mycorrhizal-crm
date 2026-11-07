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
const PAGE_LIMIT_CEILING = 100; // a bounded page — offset pagination, not "give me everything"
const seededIds: number[] = [];

test.describe('Contact list pagination at scale', { tag: '@perf-heavy' }, () => {
  // Intentional #556 tier gate (nightly schedule sets RUN_PERF_HEAVY=1), NOT a
  // missing-tool skip -- exempt from requireCiTool (issue #1315 audit).
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

  test('paging by offset keeps the DOM bounded per page and heap flat, with no duplicate rows across pages (#2188)', async ({
    page,
  }) => {
    const contactsRequests: { offset: number; limit: number | null; hasCursor: boolean }[] = [];
    page.on('request', (req) => {
      const url = req.url();
      if (req.method() === 'GET' && /\/api\/v1\/contacts\?/.test(url)) {
        const params = new URL(url).searchParams;
        contactsRequests.push({
          offset: Number(params.get('offset') ?? 0),
          limit: params.has('limit') ? Number(params.get('limit')) : null,
          hasCursor: params.has('cursor'),
        });
      }
    });

    await page.goto('/contacts?has_contact_info=false&size=25');
    await waitForLoading(page);

    const heap = () =>
      page.evaluate(
        () =>
          (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory
            ?.usedJSHeapSize ?? 0,
      );
    const heapStart = await heap();

    const cardCount = () => page.getByTestId('contact-card').count();
    const cardIds = () =>
      page
        .getByTestId('contact-card')
        .evaluateAll((els) => els.map((el) => el.getAttribute('data-contact-id')));
    const startCards = await cardCount();
    expect(startCards).toBeGreaterThan(0);
    expect(startCards).toBeLessThanOrEqual(25);

    const next = page.getByRole('button', { name: 'Go to next page' });
    const seen = new Set<string | null>(await cardIds());
    let clicks = 0;
    while ((await next.isEnabled()) && clicks < 12) {
      await next.click();
      await expect(page).toHaveURL(new RegExp(`page=${clicks + 2}`));
      await waitForLoading(page);
      const ids = await cardIds();
      // One page is rendered at a time: the DOM never accumulates.
      expect(ids.length).toBeLessThanOrEqual(25);
      for (const id of ids) {
        expect(seen.has(id), `row ${id} repeated across pages`).toBeFalsy();
        seen.add(id);
      }
      clicks++;
    }

    expect(clicks, 'expected several pages of seeded data').toBeGreaterThanOrEqual(3);

    // Every contacts request asked for a bounded page and an offset, never a
    // cursor (the two are mutually exclusive server-side).
    for (const r of contactsRequests) {
      expect(r.limit === null || r.limit <= PAGE_LIMIT_CEILING).toBeTruthy();
      expect(r.hasCursor).toBe(false);
    }
    const offsets = new Set(contactsRequests.map((r) => r.offset));
    for (let i = 0; i <= clicks; i++) expect(offsets.has(i * 25)).toBeTruthy();

    const endCards = await cardCount();

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
