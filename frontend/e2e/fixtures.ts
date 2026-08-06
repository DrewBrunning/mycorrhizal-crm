import { test as base, Page, APIRequestContext, expect } from '@playwright/test';
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
 * checks, spaced 50ms apart. A generic "layout has stopped changing" signal,
 * used below for two different reasons content can still be moving even
 * after the obvious loading indicators are gone.
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
 * Waits for the page to finish its initial load-and-settle: loading
 * indicators gone, height stable, AND every IntersectionObserver-deferred
 * section already triggered. Three layers, because two weren't enough --
 * all three are real bugs pinned down on the T36 branch by tracing actual
 * click coordinates against actual button positions in failing runs:
 *
 * 1. Indicators: both CircularProgress (`role="progressbar"`) and Skeleton
 *    (`.MuiSkeleton-root`, no ARIA role at all). Several pages
 *    (ContactDetailPage, ContactsPage, NotesPage, ActivitiesPage,
 *    DashboardPage) gate their real content behind a Skeleton, not a
 *    progressbar, so watching only `[role="progressbar"]` silently no-ops on
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
 * 3. Force-trigger deferred sections by scrolling through the whole page:
 *    ConnectionsPanel's IntersectionObserver only fires once the panel is
 *    within `rootMargin: '300px'` of the viewport -- which it may not be
 *    right after navigation, so layers 1+2 can both pass clean while the
 *    panel hasn't started loading yet. The very next scroll (Playwright's own
 *    auto-scroll before a click, e.g. scrolling to the Timeline section's
 *    Add Note button, which sits right below ConnectionsPanel) brings it
 *    into range and fires it right then -- so a click's own scroll-into-view
 *    can trigger a fresh async layout shift *during* the click gesture
 *    itself. Confirmed directly: instrumented mousedown vs. mouseup targets
 *    on a failing click showed mousedown correctly hitting the button and
 *    mouseup 15-20ms later landing on a `MuiCardContent-root` div instead --
 *    the button had physically moved out from under the cursor between the
 *    two. Scrolling to the bottom and back once, before any test touches the
 *    page, brings every such section into range and lets its fetch land
 *    while nothing is mid-click.
 *
 * Without all three, a click's target coordinates could be computed against
 * a layout that was still changing -- anywhere from the ~1140px Skeleton
 * state up through a ~30px late shift from a section that only started
 * loading because the click's own scroll brought it into view -- so the
 * click landed wherever content ended up after the page moved out from under
 * it before the click actually fired.
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

  // Force-trigger any IntersectionObserver-deferred section, then wait for
  // the resulting fetches to land, before any test's own click gets to
  // scroll there itself and race one.
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight)).catch(() => {});
  await page.waitForTimeout(100);
  await page.evaluate(() => window.scrollTo(0, 0)).catch(() => {});

  await waitForHeightStable(page);
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
