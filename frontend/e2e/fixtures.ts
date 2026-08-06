import { test as base, Page, Locator, APIRequestContext, expect } from '@playwright/test';
import { TEST_USER, API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';
import { toContactRecordInput } from '../src/api/contacts';

export { test } from '@playwright/test';
export { expect } from '@playwright/test';

export const LOGGED_OUT = { cookies: [], origins: [] };

/**
 * Logs a user in through the UI. Only needed by specs that explicitly start
 * logged out (the shared storageState already covers the common case).
 */
export async function loginUser(page: Page, credentials = TEST_USER): Promise<void> {
  await page.goto('/');

  // Already authenticated (e.g. shared storageState is active) — nothing to do.
  const dashboardHeading = page.getByRole('heading', { name: /dashboard/i });
  if (await dashboardHeading.isVisible({ timeout: 1000 }).catch(() => false)) {
    return;
  }

  await page.getByLabel(/username or email/i).fill(credentials.username);
  await page.getByLabel(/password/i).fill(credentials.password);
  await page.getByRole('button', { name: /login/i }).click();

  await expect(dashboardHeading).toBeVisible({ timeout: 15000 });
}

/**
 * Logs the current user out via the UI and waits for the login form.
 */
export async function logoutUser(page: Page): Promise<void> {
  await page.getByRole('button', { name: /logout/i }).click();
  await expect(page.getByRole('heading', { name: /login/i })).toBeVisible({ timeout: 10000 });
}

/**
 * Waits for a page's scrollHeight to hold steady across a few consecutive
 * checks, spaced 50ms apart. A generic "layout has stopped changing" signal.
 */
async function waitForHeightStable(page: Page, timeoutMs = 5000): Promise<void> {
  let lastHeight = -1;
  let stableChecks = 0;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline && stableChecks < 3) {
    const height = await page.evaluate(() => document.documentElement.scrollHeight).catch(() => -1);
    if (height === lastHeight) {
      stableChecks++;
    } else {
      stableChecks = 0;
      lastHeight = height;
    }
    await page.waitForTimeout(50);
  }
}

/**
 * Waits for the page to finish its initial load: loading indicators gone,
 * then height holding steady. Two layers, because watching indicators alone
 * silently no-ops on several pages -- a real bug pinned down on the T36
 * branch by tracing actual click coordinates against actual button
 * positions in a failing CI run:
 *
 * 1. Indicators: both CircularProgress (`role="progressbar"`) and Skeleton
 *    (`.MuiSkeleton-root`, no ARIA role at all). ContactDetailPage,
 *    ContactsPage, NotesPage, ActivitiesPage and DashboardPage all gate
 *    their real content behind a Skeleton, not a progressbar, so watching
 *    only `[role="progressbar"]` (the old implementation) silently no-ops on
 *    them: `waitForSelector(..., {state: 'hidden'})` resolves instantly when
 *    the selector never matches anything, which is exactly what happens when
 *    the only progressbar on the page is App.tsx's per-route
 *    `<Suspense fallback={<CircularProgress/>}>` -- dead weight, since every
 *    page is a plain eager `import`, never `React.lazy()`, so Suspense never
 *    actually suspends on it.
 *
 * 2. Height stability: closes a gap layer 1 structurally cannot -- a section
 *    can defer its own fetch (e.g. ContactDetailPage's ConnectionsPanel,
 *    gated behind an IntersectionObserver) and render nothing at all, `null`,
 *    until that fetch starts. There is no indicator to watch for during that
 *    window; the only real signal is that the page is about to grow once the
 *    fetch lands.
 *
 * This much closes the *initial-load* race, but does not by itself make
 * every click safe: a section like ConnectionsPanel can still be triggered
 * fresh by a *later* scroll (e.g. a click's own scroll-into-view bringing it
 * within its `rootMargin`), shifting the page again well after this
 * function has already returned. That is a per-click race, not a page-load
 * one, and closing it here (e.g. by scrolling through the whole page first)
 * was tried and measured to still fail -- see `stableClick` below, which is
 * the layer that actually closes it, by re-checking immediately before each
 * click instead of trying to predict every async side effect in advance.
 */
export async function waitForLoading(page: Page): Promise<void> {
  await page
    .waitForFunction(
      () => {
        const isVisible = (el: Element) => {
          const rect = el.getBoundingClientRect();
          return rect.width > 0 && rect.height > 0;
        };
        return ![...document.querySelectorAll('[role="progressbar"], .MuiSkeleton-root')].some(isVisible);
      },
      { timeout: 10000 }
    )
    .catch(() => {});

  await waitForHeightStable(page);
}

/**
 * Clicks a locator only once its on-screen position has held steady --
 * closes a click-time race `waitForLoading` structurally can't (see its own
 * doc comment): a section can still shift the page *during* a click's own
 * scroll-into-view, well after page load already settled, moving the target
 * out from under the cursor between mousedown and mouseup. Confirmed
 * directly on ContactDetailPage's Add Note button: instrumenting mousedown
 * vs. mouseup targets on a failing click showed mousedown correctly hitting
 * the button and mouseup 15-20ms later landing on a `MuiCardContent-root`
 * div instead -- the button had physically moved in that window. Waiting
 * for the *specific target's* position to stop changing immediately before
 * clicking closes this regardless of what's causing the shift, which
 * proved more reliable in practice than trying to preemptively trigger
 * every possible async side effect during `waitForLoading`.
 *
 * Prefer this over a plain `.click()` for any button that opens a dialog on
 * a page with dynamically-loaded sections above it (ContactDetailPage's
 * Add Note / Add Activity / Add Life Event and its life-event edit
 * affordance are the ones this was pinned down against).
 */
export async function stableClick(locator: Locator): Promise<void> {
  const page = locator.page();
  let lastPosition: { x: number; y: number } | null = null;
  let stableChecks = 0;
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline && stableChecks < 3) {
    // Explicit per-call timeouts, not just the loop's own deadline: without
    // one, scrollIntoViewIfNeeded/boundingBox on a locator that never
    // resolves (e.g. an option in a dropdown that isn't actually open --
    // its own real bug, not something this loop can wait out) hangs inside
    // that single await, past this loop's 5s budget, until the *test's*
    // outer timeout eventually force-closes the page -- which reports a
    // useless "Target page ... has been closed" instead of the real error.
    await locator.scrollIntoViewIfNeeded({ timeout: 2000 }).catch(() => {});
    const box = await locator.boundingBox({ timeout: 2000 }).catch(() => null);
    if (box && lastPosition && box.x === lastPosition.x && box.y === lastPosition.y) {
      stableChecks++;
    } else {
      stableChecks = 0;
    }
    lastPosition = box ? { x: box.x, y: box.y } : null;
    await page.waitForTimeout(50);
  }
  await locator.click();
}

/**
 * Searches the contacts list and returns once the matching contact is visible.
 * Replaces the previous fill + fixed-sleep pattern with an auto-waiting assertion.
 */
export async function searchContact(page: Page, query: string): Promise<void> {
  const searchInput = page.locator('input[placeholder*="earch"]').first();
  await searchInput.fill(query);
  await searchInput.press('Enter');
}

// ---------------------------------------------------------------------------
// API helpers — used to set up and tear down data without driving the UI.
// ---------------------------------------------------------------------------

export interface CreatedContact {
  ID: number;
  uid: string;
  firstname: string;
  lastname: string;
}

/**
 * Creates a throwaway contact via the API. Names are prefixed so global-setup
 * can sweep up any that leak when a test crashes mid-run.
 */
export async function createTestContact(
  request: APIRequestContext,
  overrides: Record<string, unknown> = {}
): Promise<CreatedContact> {
  const firstname = (overrides.firstname as string | undefined) ?? `${E2E_CONTACT_PREFIX}${Date.now()}`;
  const lastname = (overrides.lastname as string | undefined) ?? 'Temp';
  const response = await request.post(`${API_BASE_URL}/contacts`, {
    data: toContactRecordInput({ firstname, lastname, ...overrides }),
  });
  expect(response.ok(), `failed to create test contact: ${response.status()}`).toBeTruthy();
  // The API wraps the created contact: { contact: {...} }. The nested
  // ContactRecordResponse doesn't carry flat firstname/lastname/ID fields
  // (see src/api/contacts.ts's toLegacyContact for the full mapping), so
  // this echoes back what was actually sent rather than re-deriving it.
  const body = await response.json();
  const created = body.contact || body;
  return { ID: created.id ?? created.ID, uid: created.uid, firstname, lastname };
}

/**
 * Deletes a contact via the API. Safe to call in finally/afterEach blocks.
 */
export async function deleteTestContact(
  request: APIRequestContext,
  id: number | string
): Promise<void> {
  await request.delete(`${API_BASE_URL}/contacts/${id}`).catch(() => {});
}
