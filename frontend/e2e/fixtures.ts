import { Page, Locator, APIRequestContext, expect } from '@playwright/test';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import * as crypto from 'crypto';
import { TEST_USER, API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';
import { toContactRecordInput } from '../src/api/contacts';

export { test } from '@playwright/test';
export { expect } from '@playwright/test';

export const LOGGED_OUT = { cookies: [], origins: [] };

// ---------------------------------------------------------------------------
// Uniqueness helpers — `Date.now()` alone is 1ms-resolution and nothing else,
// so two tests racing under `fullyParallel` (different worker processes, but
// scheduled close enough in wall-clock time) can generate the identical
// "unique" token. Mixing in Math.random() plus a per-process monotonic
// counter means two callers in the very same millisecond still diverge.
// ---------------------------------------------------------------------------
let uniqueCounter = 0;

/**
 * A short, collision-resistant token safe to embed in a name/email/username.
 * Base-36 rather than decimal specifically to stay compact: it feeds
 * makeThrowawayUser() below, and User.Username validates max=50, so a
 * verbose label (e.g. "isolation_share_thirdparty") plus the "e2e_" prefix
 * and separators already spends over half that budget before the token.
 */
export function uniqueToken(): string {
  uniqueCounter += 1;
  const randomPart = crypto.randomBytes(4).toString('base64url').slice(0, 6).toLowerCase();
  return `${Date.now().toString(36)}${randomPart}${uniqueCounter.toString(36)}`;
}

/**
 * A numeric-only unique suffix, for fields that must look like a phone
 * number (search.spec.ts's cross-format phone tests). Longer than `length`
 * internally so the requested tail is still unique even after truncation.
 */
export function uniqueDigits(length = 10): string {
  uniqueCounter += 1;
  const randomPart = crypto.randomInt(1_000_000_000).toString().padStart(9, '0');
  const raw = `${Date.now()}${randomPart}${uniqueCounter}`;
  return raw.slice(-length);
}

// ---------------------------------------------------------------------------
// Cross-process mutex for the shared TEST_USER's account-level settings
// (/users/enabled-contact-fields, /users/date-format, /notifications/config,
// ...). These are singleton rows on the one account every spec authenticates
// as (see playwright.config.ts's storageState), so two specs that each
// read-modify-write one of them -- even from different files -- can
// interleave their toggle/restore under `fullyParallel: true` and corrupt
// each other's setting mid-test.
//
// `test.describe.configure({ mode: 'serial' })` does NOT close this gap: it
// only serializes tests *within* one file, and each Playwright worker is a
// separate OS process, so an in-memory JS lock wouldn't reach across workers
// either. This is a real cross-process lock -- an atomic `fs.mkdirSync` on a
// well-known directory in the OS temp dir -- specifically because the race
// that motivated it (dateFormats.spec.ts and linkFieldTypeEditors.spec.ts
// both mutating /users/enabled-contact-fields with no coordination) is a
// cross-file, cross-worker race.
const USER_SETTINGS_LOCK_DIR = path.join(os.tmpdir(), 'mycorrhizal-e2e-user-settings.lock');
const USER_SETTINGS_LOCK_STALE_MS = 45_000;

async function acquireUserSettingsLock(timeoutMs = 60_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      fs.mkdirSync(USER_SETTINGS_LOCK_DIR);
      return;
    } catch (err: unknown) {
      if ((err as NodeJS.ErrnoException).code !== 'EEXIST') throw err;

      // Break a lock abandoned by a crashed test/process rather than hang
      // every later test forever.
      try {
        const stat = fs.statSync(USER_SETTINGS_LOCK_DIR);
        if (Date.now() - stat.mtimeMs > USER_SETTINGS_LOCK_STALE_MS) {
          fs.rmSync(USER_SETTINGS_LOCK_DIR, { recursive: true, force: true });
          continue;
        }
      } catch {
        // Lock dir vanished between the failed mkdir and this check -- the
        // holder just released it. Retry the mkdir immediately.
        continue;
      }

      if (Date.now() > deadline) {
        throw new Error(
          'Timed out waiting for the shared e2e user-settings lock -- another test is holding it ' +
          `(${USER_SETTINGS_LOCK_DIR}). If that directory is stale from a crashed run, delete it.`
        );
      }
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
}

function releaseUserSettingsLock(): void {
  fs.rmSync(USER_SETTINGS_LOCK_DIR, { recursive: true, force: true });
}

/**
 * Runs `fn` with exclusive access to the shared TEST_USER's account-level
 * settings. Every spec that reads-modifies-writes a singleton per-user
 * setting (enabled-contact-fields, date-format, notification config, ...)
 * and restores it afterward must wrap its whole test body in this, so its
 * toggle-then-restore window can never interleave with another spec's.
 */
export async function withExclusiveUserSettings<T>(fn: () => Promise<T>): Promise<T> {
  await acquireUserSettingsLock();
  try {
    return await fn();
  } finally {
    releaseUserSettingsLock();
  }
}

// ---------------------------------------------------------------------------
// Throwaway secondary accounts (isolation checks, contact-share recipients,
// ...) — always uniquely suffixed so two tests (or two runs) never fight over
// the same account, and always cleaned up via the admin API so nothing
// accumulates in the shared database across runs.
// ---------------------------------------------------------------------------
export interface ThrowawayUser {
  username: string;
  email: string;
  password: string;
}

/** Builds a never-reused throwaway account. Does not register it. */
export function makeThrowawayUser(label: string): ThrowawayUser {
  const token = uniqueToken();
  return {
    username: `e2e_${label}_${token}`,
    email: `e2e_${label}_${token}@example.com`,
    password: 'ThrowawayPass123!',
  };
}

/**
 * Deletes a throwaway account by username via the admin API. The shared
 * TEST_USER is auto-admin (the first registered account -- see
 * userManagement.spec.ts's note), and DeleteUser is a real hard delete (the
 * one deliberate exception in CLAUDE.md's soft/hard-delete rules, precisely
 * so a torn-down account's username/email can be reused), so this leaves
 * nothing for a later run to skip over.
 *
 * `adminRequest` must be an authenticated-as-TEST_USER request context (e.g.
 * `page.request` on a page using the shared storageState) -- never the
 * throwaway account's own context.
 */
export async function deleteThrowawayUser(
  adminRequest: APIRequestContext,
  username: string
): Promise<void> {
  try {
    const directory = await adminRequest.get(`${API_BASE_URL}/users/directory`);
    if (!directory.ok()) return;
    const { users } = await directory.json();
    const match = (users || []).find((u: { id: number; username: string }) => u.username === username);
    if (match) {
      await adminRequest.delete(`${API_BASE_URL}/admin/users/${match.id}`).catch(() => {});
    }
  } catch {
    // Best-effort cleanup; never fail a test over it.
  }
}

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
 * #211: BulkActionsBar's "N selected" copy is now echoed into the app's
 * `aria-live="polite"` announcer region too (by design -- see the model
 * comment block at the top of App.tsx), so a plain `page.getByText('N
 * selected')` hits a Playwright strict-mode violation (two matches) the
 * moment the live region's text has updated. The visible bar renders its
 * copy as a `<p>`; the announcer region is a `<div aria-live="polite">` --
 * scope to the former.
 */
export function selectedText(page: Page, text: string | RegExp): Locator {
  return page.locator('p').filter({ hasText: text });
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
  const firstname = (overrides.firstname as string | undefined) ?? `${E2E_CONTACT_PREFIX}${uniqueToken()}`;
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
