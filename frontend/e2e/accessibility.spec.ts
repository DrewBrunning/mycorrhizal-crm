import type { Page } from '@playwright/test';
import { request as apiRequest } from '@playwright/test';
import {
  assertNoBlockingA11yViolations,
  createTestContact,
  deleteTestContact,
  expect,
  LOGGED_OUT,
  SKIP_A11Y_SCAN,
  test,
  waitForLoading,
} from './fixtures';
import { API_BASE_URL } from './global-setup';

/**
 * Automated accessibility smoke gate (issue #195). Landed on top of the WCAG
 * 2.2 audit (#148) so the *machine-catchable* half of its findings stay out:
 * axe-core scans every route in both themes for critical/serious violations,
 * and eslint-plugin-jsx-a11y catches the authoring-time class (unnamed icon
 * buttons, unlabeled form controls, ...) the same way TypeScript catches
 * types.
 *
 * Only `critical` and `serious` impact block. The audit also found `moderate`
 * findings (`region`, `page-has-heading-one`, `heading-order`, #211); tighten
 * the filter to include them once those fixes land.
 *
 * Every test below already ends with an explicit, targeted
 * `assertNoBlockingA11yViolations` call (scoped to a route, theme, or dialog
 * subtree) -- fixtures.ts's automatic per-test scan (issue #259) would just
 * re-scan the exact same final page state a second time for zero added
 * coverage, so this whole file opts out of it.
 */

// ---------------------------------------------------------------------------
// Coverage inventory
// ---------------------------------------------------------------------------
// 15 authenticated routes x 2 themes + login/register (logged out) x 2 themes
// = 34 route scans. `:contactId` is resolved to a real contact at runtime via
// beforeAll (hardcoding an id like /contacts/2 is fragile: seeded-contact ids
// shift with the schema, and parallel specs churn the shared account's data).
const AUTH_ROUTES: Array<[string, string]> = [
  ['dashboard', '/'],
  ['contacts', '/contacts'],
  ['contact-detail', '/contacts/:contactId'],
  ['prep', '/contacts/:contactId/prep'],
  ['notes', '/notes'],
  ['activities', '/activities'],
  ['network', '/network'],
  ['households', '/households'],
  ['circles', '/circles'],
  ['shares', '/shares'],
  ['audit', '/audit'],
  ['users', '/users'],
  ['settings', '/settings'],
  ['data-settings', '/settings/data'],
  ['triage', '/circle-tag-triage'],
];

const LOGGED_OUT_ROUTES: Array<[string, string]> = [
  ['login', '/login'],
  ['register', '/register'],
];

const THEMES = ['light', 'dark'] as const;
type Theme = (typeof THEMES)[number];

// assertNoBlockingViolations here is just fixtures.ts's shared
// assertNoBlockingA11yViolations under its old local name — scanning only
// the dialog (not the page it sits over) when `context` is passed is
// implemented there now, shared with the automatic per-test scan so the two
// can never define "blocking" differently.
const assertNoBlockingViolations = assertNoBlockingA11yViolations;

async function gotoWithTheme(page: Page, route: string, theme: Theme): Promise<void> {
  await page.addInitScript((t) => localStorage.setItem('themePreference', t), theme);
  await page.goto(route);
  await waitForLoading(page);
}

// Resolve a contact id for the /contacts/:contactId routes. The seeded
// "Alice Johnson" contact is upserted by global-setup every run and never
// deleted by other specs; fall back to creating one if it is somehow absent.
//
// Uses an explicitly authenticated API context, NOT the `request` fixture:
// when only a logged-out test is selected (e.g. `-g "login (light)"`),
// Playwright also applies that describe's `test.use({ storageState: LOGGED_OUT })`
// to the file-level beforeAll's `request` fixture, making it 401. An explicit
// context is deterministic regardless of the selection.
const AUTH_STORAGE_STATE = 'playwright/.auth/user.json';

let contactId: number | undefined;
let contactCreated = false;

test.beforeAll(async () => {
  const ctx = await apiRequest.newContext({
    baseURL: 'http://localhost:7300',
    storageState: AUTH_STORAGE_STATE,
  });
  try {
    const search = await ctx.get(`${API_BASE_URL}/contacts?search=Alice&limit=1`);
    if (search.ok()) {
      const { contacts } = await search.json();
      const alice = (contacts || []).find(
        (c: { firstname: string; lastname: string }) =>
          c.firstname === 'Alice' && c.lastname === 'Johnson',
      );
      if (alice) {
        contactId = alice.id;
        return;
      }
    }
    const created = await createTestContact(ctx, { firstname: 'Alice', lastname: 'Johnson' });
    contactId = created.ID;
    contactCreated = true;
  } finally {
    await ctx.dispose();
  }
});

test.afterAll(async () => {
  // Only delete the contact we created ourselves — never the seeded one.
  if (contactCreated && contactId) {
    const ctx = await apiRequest.newContext({
      baseURL: 'http://localhost:7300',
      storageState: AUTH_STORAGE_STATE,
    });
    try {
      await deleteTestContact(ctx, contactId);
    } finally {
      await ctx.dispose();
    }
  }
});

test.describe('accessibility route scans', {
  annotation: { type: SKIP_A11Y_SCAN, description: 'already scans explicitly per route/theme' },
}, () => {
  test.describe('authenticated routes', () => {
    for (const theme of THEMES) {
      for (const [name, route] of AUTH_ROUTES) {
        test(`${name} (${theme}) has no critical or serious a11y violations`, async ({ page }) => {
          const resolved = route.replace(':contactId', String(contactId));
          await gotoWithTheme(page, resolved, theme);
          await assertNoBlockingViolations(page);
        });
      }
    }
  });

  test.describe('logged-out routes', () => {
    test.use({ storageState: LOGGED_OUT });
    for (const theme of THEMES) {
      for (const [name, route] of LOGGED_OUT_ROUTES) {
        test(`${name} (${theme}) has no critical or serious a11y violations`, async ({ page }) => {
          await gotoWithTheme(page, route, theme);
          await assertNoBlockingViolations(page);
        });
      }
    }
  });
});

test.describe('accessibility dialog scans', {
  annotation: { type: SKIP_A11Y_SCAN, description: 'already scans explicitly per dialog/theme' },
}, () => {
  // The audit's first pass only scanned page loads and missed every dialog.
  // Scan the dialog element itself (not the page behind it), in both themes —
  // color-contrast findings are theme-dependent, so a light-only pass could
  // still ship a dark-mode regression.
  for (const theme of THEMES) {
    test(`add contact dialog (${theme}) has no critical or serious a11y violations`, async ({
      page,
    }) => {
      await gotoWithTheme(page, '/contacts', theme);
      await page.getByRole('button', { name: /add/i }).click();
      await expect(page.getByRole('dialog')).toBeVisible();
      await assertNoBlockingViolations(page, '[role="dialog"]');
    });

    test(`import dialog (${theme}) has no critical or serious a11y violations`, async ({
      page,
    }) => {
      await gotoWithTheme(page, '/contacts', theme);
      await page.getByRole('button', { name: /import/i }).click();
      await expect(page.getByRole('dialog')).toBeVisible();
      await assertNoBlockingViolations(page, '[role="dialog"]');
    });

    test(`review duplicates dialog (${theme}) has no critical or serious a11y violations`, async ({
      page,
    }) => {
      await gotoWithTheme(page, '/contacts', theme);
      await page.getByRole('button', { name: /review duplicates/i }).click();
      await expect(page.getByRole('dialog')).toBeVisible();
      await assertNoBlockingViolations(page, '[role="dialog"]');
    });

    test(`contact edit form (${theme}) has no critical or serious a11y violations`, async ({
      page,
    }) => {
      await gotoWithTheme(page, `/contacts/${contactId}`, theme);
      // Unlike the other dialogs, profile editing here is an inline form in
      // the contact header (no [role="dialog"]), so scan the document with
      // edit mode active — the equivalent state for the audit's blind spot.
      await page.locator('.edit-icon').first().click();
      await expect(page.getByRole('button', { name: 'Save' })).toBeVisible();
      await assertNoBlockingViolations(page);
    });
  }
});
