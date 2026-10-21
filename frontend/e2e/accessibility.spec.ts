import type { Page } from '@playwright/test';
import { request as apiRequest } from '@playwright/test';
import type { SystemStatusResponse } from '../src/api/systemStatus';
import {
  assertNoAaaContrastViolations,
  assertNoAxeRuleViolations,
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
// 16 authenticated routes x 2 themes + login/register (logged out) x 2 themes
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
  ['map', '/map'],
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

// The scans look up the shared `testuser`'s seeded Alice (beforeAll below), so
// they authenticate as that same user rather than a per-worker one (issue #1480).
test.use({ sharedUser: true });

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

// ---------------------------------------------------------------------------
// WCAG AAA checks (issue #964)
// ---------------------------------------------------------------------------
// The palette claims AAA (7:1) for specific documented token pairs, not for
// every string the app renders. The deterministic half of that claim is
// unit-tested in src/themeAccessibility.test.ts; these two blocks add the
// rendered-text half -- the brand AppBar surface, whose white-on-brand-green
// pair is claimed AAA, and the global reduced-motion (2.3.3) behaviour. The
// app-wide axe gate above stays AA by design: secondary text on parchment is
// 6.35:1 (light) / 6.78:1 (dark), deliberately AA-only.

test.describe('WCAG AAA contrast (documented brand surface)', {
  annotation: {
    type: SKIP_A11Y_SCAN,
    description: 'scans the brand surface explicitly with the AAA rule',
  },
}, () => {
  for (const theme of THEMES) {
    test(`app bar brand surface (${theme}) has no AAA contrast violations`, async ({ page }) => {
      await gotoWithTheme(page, '/', theme);
      // The AppBar is pinned to a fixed brand green with white text in both
      // modes (theme.ts's MuiAppBar override) -- the one rendered surface the
      // palette claims AAA for.
      await assertNoAaaContrastViolations(page, 'header');
    });
  }
});

/**
 * Reads the computed motion styles of a freshly-injected probe element. Inline
 * values are used so the probe depends on no app class name; the theme's
 * reduced-motion override uses `!important`, which beats an author inline
 * declaration, so under the preference the probe reports the collapsed values.
 */
async function probeMotion(page: Page): Promise<{
  transitionMs: number;
  animationMs: number;
  scrollBehavior: string;
}> {
  return page.evaluate(() => {
    const el = document.createElement('div');
    el.style.transitionDuration = '250ms';
    el.style.animationDuration = '250ms';
    el.style.scrollBehavior = 'smooth';
    document.body.appendChild(el);
    const style = getComputedStyle(el);
    const toMs = (value: string) =>
      value.endsWith('ms') ? parseFloat(value) : parseFloat(value) * 1000;
    const result = {
      transitionMs: toMs(style.transitionDuration),
      animationMs: toMs(style.animationDuration),
      scrollBehavior: style.scrollBehavior,
    };
    el.remove();
    return result;
  });
}

test.describe('WCAG 2.3.3 reduced motion (AAA)', {
  annotation: {
    type: SKIP_A11Y_SCAN,
    description: 'asserts computed motion styles, not axe findings',
  },
}, () => {
  for (const theme of THEMES) {
    test(`prefers-reduced-motion collapses transitions and animations (${theme})`, async ({
      page,
    }) => {
      await gotoWithTheme(page, '/', theme);

      // Control: without the preference the probe's inline values stand.
      const normal = await probeMotion(page);
      expect(normal.transitionMs).toBeCloseTo(250, 0);
      expect(normal.animationMs).toBeCloseTo(250, 0);
      expect(normal.scrollBehavior).toBe('smooth');

      await page.emulateMedia({ reducedMotion: 'reduce' });
      expect(
        await page.evaluate(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches),
      ).toBe(true);

      const reduced = await probeMotion(page);
      expect(reduced.transitionMs).toBeLessThan(1);
      expect(reduced.animationMs).toBeLessThan(1);
      expect(reduced.scrollBehavior).toBe('auto');
    });
  }
});

// ---------------------------------------------------------------------------
// Status text, scroll regions and landmarks (issues #1792, #1788)
// ---------------------------------------------------------------------------
// The route scans above never rendered a degraded/failing status, a
// relationship-health "On track"/overdue line or a horizontally scrolling table,
// which is how 1.83:1 "Degraded" text and an unreachable scroll region shipped.
// These specs render those states on purpose (real backend data where it can be
// made, route-patched where a state cannot be produced on demand), in both
// themes, and scan them with the same blocking gate.

test.describe('status text and landmarks (#1792)', {
  annotation: { type: SKIP_A11Y_SCAN, description: 'scans each seeded state explicitly' },
}, () => {
  for (const theme of THEMES) {
    test(`system status with degraded/unhealthy/healthy chips (${theme})`, async ({ page }) => {
      // The real snapshot is "healthy", so stub one that renders every chip
      // colour: success (OK), warning (Degraded), error (Unhealthy), default
      // (Not configured).
      const snapshot: SystemStatusResponse = {
        overall: 'degraded',
        health: {
          status: 'degraded',
          database: { status: 'ok' },
          migrations: { status: 'ok' },
          integrity_check: { status: 'unhealthy', reason: 'check failed' },
          restore_drill: { status: 'degraded', reason: 'drill is overdue' },
          background_jobs: { status: 'ok', jobs: [] },
          integrations: {
            immich: { status: 'ok' },
            webdav: { status: 'degraded', reason: 'slow' },
            oidc: { status: 'not_configured' },
          },
        },
        version: { version: '1.5.0', commit: 'abc1234', build_date: '2026-10-01T00:00:00Z' },
        uptime: { started_at: '2026-10-01T00:00:00Z', uptime_seconds: 3600 },
        migration: { applied: 70, latest: 70, pending: 0, dirty: false },
        config: {
          validation: [],
          features: {
            carddav: true,
            caldav: false,
            oidc: false,
            email: false,
            metrics: false,
            db_integrity_check: true,
            db_restore_drill: true,
          },
        },
        database: { sqlite_version: '3.46.0', journal_mode: 'wal', wal_bytes: 0 },
        storage: {
          database_bytes: 1024,
          filesystem: { free_bytes: 1_000_000, total_bytes: 2_000_000 },
          directories: [],
          threshold: 'ok',
        },
        update: { enabled: false },
      };
      await page.route('**/api/v1/admin/system-status', (route) =>
        route.fulfill({ json: snapshot }),
      );
      // Not gotoWithTheme: waitForLoading's height-stability poll never settles
      // on this page under the Playwright harness (page.evaluate stalls), and the
      // existing systemEvents.spec.ts likewise skips it for /system-status.
      await page.addInitScript((t) => localStorage.setItem('themePreference', t), theme);
      await page.goto('/system-status');
      await expect(page.getByText('Overall status')).toBeVisible();
      await expect(page.getByText('Degraded').first()).toBeVisible();
      await assertNoBlockingViolations(page);
    });

    test(`system events with warn/error/failure chips (${theme})`, async ({ page }) => {
      const base = {
        created_at: '2026-10-01T10:00:00Z',
        occurred_at: '2026-10-01T10:00:00Z',
        component: 'scheduler',
        correlation_id: 'corr-1',
      };
      await page.route('**/api/v1/admin/system-events?*', (route) =>
        route.fulfill({
          json: {
            total: 3,
            system_events: [
              { ...base, id: 3, event_type: 'job_completed', severity: 'info', result: 'success' },
              { ...base, id: 2, event_type: 'job_failed', severity: 'warn', result: 'skipped' },
              {
                ...base,
                id: 1,
                event_type: 'job_failed',
                severity: 'error',
                result: 'failure',
                error: 'boom',
              },
            ],
          },
        }),
      );
      await gotoWithTheme(page, '/system-events', theme);
      await expect(page.getByRole('table')).toBeVisible();
      await assertNoBlockingViolations(page);
    });

    test(`relationship health "On track" and overdue text (${theme})`, async ({
      page,
      request,
    }) => {
      const onTrack = await createTestContact(request, { firstname: 'A11yOnTrack' });
      const overdue = await createTestContact(request, { firstname: 'A11yOverdue' });
      try {
        const daysAgo = (n: number) => new Date(Date.now() - n * 86_400_000).toISOString();
        for (const [contact, date] of [
          [onTrack, daysAgo(1)],
          [overdue, daysAgo(90)],
        ] as const) {
          const activity = await request.post(`${API_BASE_URL}/activities`, {
            data: { title: 'Coffee', type: 'visit', date, contact_ids: [contact.ID] },
          });
          expect(activity.ok(), `activity: ${await activity.text()}`).toBeTruthy();
          const cadence = await request.post(`${API_BASE_URL}/cadence-policies`, {
            data: { entity_id: contact.uid, target_interval_days: 30 },
          });
          expect(cadence.ok(), `cadence: ${await cadence.text()}`).toBeTruthy();
        }

        for (const route of [`/contacts/${onTrack.ID}`, `/contacts/${onTrack.ID}/prep`]) {
          await gotoWithTheme(page, route, theme);
          await expect(page.getByText('On track').first()).toBeVisible({ timeout: 15000 });
          await assertNoBlockingViolations(page);
        }
        for (const route of [`/contacts/${overdue.ID}`, `/contacts/${overdue.ID}/prep`]) {
          await gotoWithTheme(page, route, theme);
          await expect(page.getByText(/days overdue/).first()).toBeVisible({ timeout: 15000 });
          await assertNoBlockingViolations(page);
        }
      } finally {
        await deleteTestContact(request, onTrack.ID);
        await deleteTestContact(request, overdue.ID);
      }
    });

    // axe's `region` rule is `moderate`/best-practice, so the app-wide gate
    // (critical/serious) never saw the sidebar logo sitting outside every
    // landmark. Default viewport is 1280px, where the permanent drawer renders.
    test(`every piece of content is inside a landmark (${theme})`, async ({ page }) => {
      for (const route of ['/', '/contacts', '/audit', '/settings']) {
        await gotoWithTheme(page, route, theme);
        await assertNoAxeRuleViolations(page, ['region']);
      }
    });
  }
});

test.describe('audit log at narrow widths (#1788, #1792)', {
  annotation: { type: SKIP_A11Y_SCAN, description: 'scans explicitly at each viewport' },
}, () => {
  test('the scrolling table region is focusable and labelled', async ({ page }) => {
    // 920px wide: the permanent drawer takes 256px, leaving less than the
    // table's 640px minimum, so the container genuinely scrolls horizontally.
    await page.setViewportSize({ width: 920, height: 900 });
    await gotoWithTheme(page, '/audit', 'light');
    const region = page.getByRole('region', { name: 'Audit log' });
    await expect(region).toBeVisible();
    await expect.poll(() => region.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(true);
    await region.focus();
    await expect(region).toBeFocused();
    await assertNoAxeRuleViolations(page, ['scrollable-region-focusable']);
  });

  for (const width of [320, 390]) {
    test(`phones (${width}px) get a readable card list with no sideways scroll`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 800 });
      await gotoWithTheme(page, '/audit', 'light');
      const list = page.getByRole('list', { name: 'Audit log' });
      await expect(list).toBeVisible();
      await expect(page.getByRole('table')).toHaveCount(0);
      // Every card shows its entity and operation chips -- not 2 of 5 columns.
      await expect(list.getByRole('listitem').first().locator('.MuiChip-root')).toHaveCount(2);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
      await assertNoBlockingViolations(page);
    });
  }
});
