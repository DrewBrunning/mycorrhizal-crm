import * as crypto from 'node:crypto';
import * as path from 'node:path';
import AxeBuilder from '@axe-core/playwright';
import {
  type APIRequestContext,
  type BrowserContextOptions,
  test as base,
  expect,
  type Locator,
  type Page,
} from '@playwright/test';
import { toContactRecordInput } from '../src/api/contacts';
import { API_BASE_URL, E2E_CONTACT_PREFIX, TEST_USER } from './global-setup';
import {
  deleteAccountAsAdmin,
  isSettingsMutation,
  provisionWorkerUser,
  SHARED_STORAGE_STATE,
  type WorkerUser,
} from './workerUser';

export { expect } from '@playwright/test';

export const LOGGED_OUT = { cookies: [], origins: [] };

// ---------------------------------------------------------------------------
// Automatic accessibility gate (issue #259). accessibility.spec.ts (#195)
// scans a curated inventory of routes/themes/dialogs on its own schedule;
// this closes the gap *between* audits by scanning whatever page every other
// spec happens to be left on after each test, so a new critical/serious
// violation is caught the day the feature that introduced it lands, not at
// the next dedicated audit sweep.
//
// Implemented by overriding the built-in `page` fixture (the documented
// Playwright pattern for wrapping a fixture) rather than a separate
// `auto: true` fixture that itself depends on `page` -- that would force
// Playwright to spin up a browser page for every test in the file,
// including the many API-only specs that destructure `{ request }` and
// never touch `page` at all (contactSort.spec.ts, search.spec.ts,
// timelineEndpoint.spec.ts, ...). Overriding `page` instead means this
// teardown only ever runs for tests that actually requested a page.
// ---------------------------------------------------------------------------

/** Blocking impact levels -- shared with accessibility.spec.ts so the two never drift. */
export const BLOCKING_A11Y_IMPACTS = ['critical', 'serious'];

/** axe-core tags mapping to the WCAG 2.0/2.1/2.2 A+AA levels the #148 audit ran. */
export const WCAG_A11Y_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

/**
 * axe's WCAG 1.4.6 Contrast (Enhanced) rule, the AAA (7:1) sibling of
 * `color-contrast`. It is disabled by default (AAA is not part of the app-wide
 * gate), so it must be opted into per call. Scoped deliberately: this palette
 * claims AAA only for the documented token pairs -- secondary text on the
 * parchment card surface is AA-only by design (6.35:1 light / 6.78:1 dark), so
 * running this rule over a whole route reports hundreds of AA-not-AAA pairs
 * that are not regressions. It is used against the brand-surface subtree, whose
 * pair *is* claimed AAA (issue #964).
 */
export const AAA_CONTRAST_RULE = 'color-contrast-enhanced';

/**
 * Annotation `type` a spec uses to opt a `test`/`test.describe` block out of
 * the automatic per-test scan below, for flows that intentionally render a
 * transient state (mid-animation toast, a deliberately-broken form left
 * showing a validation error, ...) where scanning whatever the page happens
 * to be doing right as the test body returns would be a false positive.
 * Scope the annotation to the smallest block that actually needs it rather
 * than disabling a whole file.
 *
 * Usage:
 *   test.describe('flow with a transient state', {
 *     annotation: { type: SKIP_A11Y_SCAN, description: 'why' },
 *   }, () => { ... });
 */
export const SKIP_A11Y_SCAN = 'skip-a11y-scan';

/**
 * Waits for any in-flight CSS transition/animation on `context` (default: the
 * whole page) to settle before a contrast-sensitive scan runs.
 *
 * Root-caused a real nightly flake (issue #1279, run 36406818436): the
 * "import dialog (light)" dialog scan only waits for
 * `page.getByRole('dialog')` to become visible, which Playwright considers
 * true as soon as the dialog is attached and non-zero-size -- not once MUI's
 * ~225ms Fade/Grow open transition finishes. Neither this repo's dev server
 * nor a manual axe-core run against the settled DOM reproduces any
 * color-contrast violation on that dialog's helper text (it measures 6.35:1,
 * comfortably over the 4.5:1 floor, matching the AA claim documented on
 * `text.secondary` in theme.ts) -- but the CI failure's own violation payload
 * reported both the foreground and background shifted several points toward
 * each other from their steady-state hex values, consistent with axe
 * sampling a still-fading-in frame rather than the resting one. None of the
 * dialog-scan tests set `prefers-reduced-motion: reduce` (only one dedicated
 * test in accessibility.spec.ts does), so the transition is genuinely
 * running when `assertNoBlockingA11yViolations` is called right after
 * `toBeVisible()`.
 *
 * `Element.getAnimations()` (Web Animations API) returns running CSS
 * transitions as well as CSS/WAAPI animations, so this covers MUI's
 * transition-based Dialog/Fade/Grow without needing to special-case them.
 *
 * Two guards keep this from hanging the scan (the bug that turned the above
 * fix into a red `Run E2E Tests` on PR #1281, job 109021190856):
 *
 * 1. Skip animations that can never settle. MUI's indeterminate
 *    `CircularProgress`/`LinearProgress`/`Skeleton` -- and a keyboard-focus
 *    ripple's `pulsate` (`TouchRipple.js`, `animation-iteration-count:
 *    infinite`) -- never fire `finished`, so awaiting one hangs forever. The
 *    failing case was `reminders.spec.ts`'s "should show reminder form
 *    fields": `Escape` closes the dialog and MUI restores focus to the
 *    trigger button; the Escape keydown had set MUI's global keyboard-modality
 *    flag, so the restored focus counted as focus-visible and started an
 *    infinite pulsate ripple. `getComputedTiming().iterations` is `Infinity`
 *    for exactly those; only finite animations can be waited out.
 * 2. Cap the wait anyway, matching the per-test wait below. A finite
 *    animation can still be interrupted or thrash, and the whole point of
 *    this helper is to settle *before* a scan, not to block it.
 *
 * Issue #1437 (the dark-mode replay of #1279) fixed two further gaps in the
 * original scope-based wait:
 *
 * 3. It scanned only the `context` subtree, but MUI applies the Dialog
 *    Fade/Grow to the *container* wrapping the `[role="dialog"]` paper -- an
 *    ancestor of that subtree -- so `root.getAnimations({ subtree: true })`
 *    never saw the open transition at all. Scan the whole document instead
 *    (the per-test scan below always did); the finite-only filter and the cap
 *    keep it bounded.
 * 4. It queried once, immediately after `toBeVisible()` -- which Playwright
 *    reports as soon as the paper is attached, before the Fade has started a
 *    frame later. Let two animation frames pass, then wait again, so a
 *    transition that begins in that window is still caught.
 */
async function waitForAnimationsToSettle(page: Page): Promise<void> {
  await page.evaluate(() => {
    const finiteAnimations = () =>
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
        .map((a) => a.finished.catch(() => {}));

    const settle = () =>
      Promise.race([
        Promise.all(finiteAnimations()),
        new Promise((resolve) => setTimeout(resolve, 500)),
      ]);

    const nextFrame = () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      );

    return settle().then(nextFrame).then(settle);
  });
}

/**
 * Runs axe-core against `page` (or, scoped via `context` -- a CSS selector,
 * typically `[role="dialog"]` -- against just that subtree) and fails on any
 * `critical`/`serious` violation. Shared by the automatic per-test check
 * below and accessibility.spec.ts's own dedicated route/theme/dialog scans,
 * so the two can never drift out of sync on what counts as blocking.
 */
export async function assertNoBlockingA11yViolations(page: Page, context?: string): Promise<void> {
  await waitForAnimationsToSettle(page);
  const builder = new AxeBuilder({ page }).withTags(WCAG_A11Y_TAGS);
  if (context) {
    builder.include(context);
  }
  const results = await builder.analyze();
  const blocking = results.violations.filter((v) => BLOCKING_A11Y_IMPACTS.includes(v.impact ?? ''));
  const detail = blocking.map((v) => ({
    id: v.id,
    impact: v.impact,
    nodes: v.nodes.length,
    firstTarget: v.nodes[0]?.target,
  }));
  expect(blocking, JSON.stringify(detail, null, 2)).toEqual([]);
}

/**
 * Runs axe's AAA contrast rule (`color-contrast-enhanced`, SC 1.4.6) against a
 * *scoped* subtree and fails on any violation. The scope is the whole point:
 * this palette's AAA claim is a token-pair claim (see AAA_CONTRAST_RULE), and
 * only the documented pair's rendered surface is scanned here. The app-wide
 * AA gate stays `assertNoBlockingA11yViolations`.
 */
export async function assertNoAaaContrastViolations(page: Page, context: string): Promise<void> {
  await waitForAnimationsToSettle(page);
  const results = await new AxeBuilder({ page })
    .withRules([AAA_CONTRAST_RULE])
    .include(context)
    .analyze();
  const detail = results.violations.flatMap((v) =>
    v.nodes.map((n) => ({ id: v.id, target: n.target, summary: n.failureSummary })),
  );
  expect(detail, JSON.stringify(detail, null, 2)).toEqual([]);
}

type StorageStateOption = BrowserContextOptions['storageState'];

/**
 * The per-worker-auth layer (issue #1480) with NO automatic a11y scan. Specs
 * that predate the scan and import `test` from here keep exactly the behavior
 * they had when they took Playwright's bare `test`; `test` below adds the scan.
 */
export const authTest = base.extend<
  { sharedUser: boolean; storageState: StorageStateOption },
  { workerUser: WorkerUser }
>({
  // Issue #1480: each worker authenticates as its OWN account (registered and
  // seeded through the API on first use, hard-deleted on teardown), so
  // account-level settings a test mutates never reach another worker and no
  // cross-process lock is needed. Specs read the credentials from here when
  // they must type them (sessionExpiry's in-place re-auth).
  workerUser: [
    async ({ browser }, use, workerInfo) => {
      const stateDir = path.join(workerInfo.project.outputDir, '.auth');
      const user = await provisionWorkerUser(browser, workerInfo.workerIndex, stateDir);
      await use(user);
      await deleteAccountAsAdmin(user.username);
    },
    { scope: 'worker', timeout: 60_000 },
  ],

  // Opt a file/describe into the shared, settings-immutable `testuser` (the
  // auto-admin) with `test.use({ sharedUser: true })`: for specs that need
  // admin rights or assert on the seeded dataset's exact contents. While
  // active, a browser request that would write an account-level setting fails
  // the test (see the `context` override), so that user's settings stay
  // immutable and nothing else's read of them can be corrupted.
  sharedUser: [false, { option: true }],

  // The default auth state is the worker's own user. Only fills in a value
  // when none was configured: a spec's `test.use({ storageState })` (LOGGED_OUT,
  // an explicit path) bypasses this override entirely, as before.
  storageState: async ({ storageState, sharedUser, workerUser }, use) => {
    if (storageState !== undefined) return use(storageState);
    await use(sharedUser ? SHARED_STORAGE_STATE : workerUser.storageStatePath);
  },

  context: async ({ context, sharedUser, storageState }, use) => {
    const violations: string[] = [];
    // Only the shared user's own session is guarded: a LOGGED_OUT /
    // other-account context in a sharedUser describe is not that user.
    const guarded = sharedUser && storageState === SHARED_STORAGE_STATE;
    if (guarded) {
      await context.route(/\/api\/v1\//, async (route) => {
        const req = route.request();
        if (isSettingsMutation(req.method(), req.url())) {
          violations.push(`${req.method()} ${req.url()}`);
          await route.abort('blockedbyclient');
          return;
        }
        await route.fallback();
      });
    }
    await use(context);
    expect(
      violations,
      "the shared read-only user's account settings are immutable -- run this spec as a per-worker user (drop test.use({ sharedUser: true }))",
    ).toEqual([]);
  },
});

export const test = authTest.extend<{ page: Page }>({
  page: async ({ page }, use, testInfo) => {
    await use(page);

    // Don't pile a second, unrelated-looking a11y failure onto a test that
    // already failed for its own reason.
    if (testInfo.status !== testInfo.expectedStatus) return;

    if (testInfo.annotations.some((a) => a.type === SKIP_A11Y_SCAN)) return;

    // API-only tests (viteBuild.spec.ts's page.request.get() calls, ...)
    // request `page` but never navigate it -- scanning about:blank is pure
    // overhead, and a page torn down mid-test errors out of AxeBuilder.
    const url = page.isClosed() ? '' : page.url();
    if (!url || url === 'about:blank' || url.startsWith('chrome-error://')) return;

    // A test that ends right after opening a Menu/Snackbar/Dialog (a
    // perfectly normal place to stop -- the assertion it cared about is
    // already satisfied) can still leave MUI's Grow/Fade/Slide transition
    // mid-flight, which axe's color-contrast check samples honestly: the
    // *animating* opacity, not the settled one. document.getAnimations()
    // covers CSS transitions as well as script-driven ones, so this waits
    // out whatever's actually still running (capped, so a genuinely
    // long-running/looping animation can't hang the scan) rather than
    // guessing a fixed delay. Pinned by contactDetailLayout.spec.ts's mobile
    // overflow-menu case, which raced this before the wait existed.
    await page
      .evaluate(() =>
        Promise.race([
          Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {}))),
          new Promise((resolve) => setTimeout(resolve, 500)),
        ]),
      )
      .catch(() => {});

    // Then wait out an *in-flight smooth scroll* -- e.g. the contact detail
    // jump nav's scrollIntoView({ behavior: 'smooth' }) (issue #614). A smooth
    // scroll is not a Web Animation, so it never shows up in
    // document.getAnimations() and the wait above returns immediately while the
    // page is still scrolling; axe then samples geometry at a random scroll
    // offset, and a position:sticky element (the jump nav) partially obscuring
    // an interactive target mid-scroll trips `target-size` at random. Resolve
    // on whichever comes first: a `scrollend` event, or the scroll position
    // holding steady across three consecutive animation frames (the fallback
    // for when nothing is scrolling at all, so `scrollend` never fires). Same
    // capped-timeout discipline as the animation wait -- a page that never
    // stops scrolling (an auto-advancing ticker) can't hang the scan.
    await page
      .evaluate(
        () =>
          new Promise<void>((resolve) => {
            const deadline = performance.now() + 1500;
            let done = false;
            const finish = () => {
              if (done) return;
              done = true;
              window.removeEventListener('scrollend', finish, true);
              resolve();
            };
            window.addEventListener('scrollend', finish, true);
            let last = `${window.scrollX},${window.scrollY}`;
            let stableFrames = 0;
            const tick = () => {
              if (done) return;
              const now = `${window.scrollX},${window.scrollY}`;
              stableFrames = now === last ? stableFrames + 1 : 0;
              last = now;
              if (stableFrames >= 3 || performance.now() > deadline) finish();
              else requestAnimationFrame(tick);
            };
            requestAnimationFrame(tick);
          }),
      )
      .catch(() => {});

    await assertNoBlockingA11yViolations(page);
  },
});

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
 * Deletes a throwaway account by username via the admin API, always as the
 * shared auto-admin `testuser` (the first registered account) -- NOT as the
 * calling test's own user, which under per-worker users (issue #1480) is an
 * ordinary non-admin account. DeleteUser is a real hard delete (the one
 * deliberate exception in CLAUDE.md's soft/hard-delete rules, precisely so a
 * torn-down account's username/email can be reused), so this leaves nothing
 * for a later run to skip over. Best-effort: cleanup never fails a test.
 */
export async function deleteThrowawayUser(username: string): Promise<void> {
  await deleteAccountAsAdmin(username);
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
        return ![...document.querySelectorAll('[role="progressbar"], .MuiSkeleton-root')].some(
          isVisible,
        );
      },
      { timeout: 10000 },
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
  overrides: Record<string, unknown> = {},
): Promise<CreatedContact> {
  const firstname =
    (overrides.firstname as string | undefined) ?? `${E2E_CONTACT_PREFIX}${uniqueToken()}`;
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
  id: number | string,
): Promise<void> {
  await request.delete(`${API_BASE_URL}/contacts/${id}`).catch(() => {});
}
