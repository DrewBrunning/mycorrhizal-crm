// e2e/a11ySweepPopulated.spec.ts
//
// Issue #2920 item 6: the populated, phone-width axe sweep.
//
// `e2e/accessibility.spec.ts` scans a curated route list as the shared admin,
// and `fixtures.ts` scans whatever state a spec happens to leave behind. Both
// are structurally blind to the round-2 re-audit's "only on a populated phone
// state" defects (#2920's table): neither ever renders a pending incoming
// share at 375px, and neither opens a contact whose timeline activity has
// participants. This spec closes that gap by reusing item 4's populated worker
// seed (`populatedSeed.ts`) and route inventory (`populatedRoutes.ts`) and
// running `assertNoBlockingA11yViolations` over every App.tsx route at both
// 1280px and 375px, in light and dark.
//
// It is deliberately nightly-only. Every route x two widths x two themes is
// 108 tests and ~2.7 minutes at `--workers=1`, too slow for the per-PR E2E
// job, so the file self-skips unless RUN_A11Y_SWEEP=1 (the same pattern as the
// @perf-heavy specs' RUN_PERF_HEAVY). `e2e-tests.yml`'s schedule-only
// `e2e-a11y-sweep` job sets it; PR/push runs never do.
//
// Red on the pre-fix tree (main 3fd29a6c8), by the two defects this sweep
// exists to catch:
//
//  * #2906: with a pending share, the phone AppBar's IconButton
//    aria-label="Open navigation" overrides the Badge's visible count, so
//    axe's `label-content-name-mismatch` (wcag21a, serious) fires on every
//    shell route at 375px.
//  * #2907: the timeline participant links are 19.9px tall. At 375px the two
//    names wrap onto separate rows, so axe's `target-size` (wcag22aa,
//    serious) reports both nodes (insufficient size and insufficient
//    neighbour offset) on /contacts/:id.
//
// Both are fixed in #2926; this spec is the regression gate that keeps the
// next populated-state case from shipping unseen.

import type { Page } from '@playwright/test';
import { loadRoutes } from '../scripts/check-e2e-routes.mjs';
import { assertNoBlockingA11yViolations, expect, SKIP_A11Y_SCAN, waitForLoading } from './fixtures';
import { awaitPinFonts, pinRenderingEnvironment } from './pinnedEnvironment';
import {
  ADMIN_ROUTES,
  expectSeededRouteContent,
  ROUTE_EXEMPTIONS,
  routeFor,
} from './populatedRoutes';
import { test } from './populatedSeed';

const RUN_A11Y_SWEEP = process.env.RUN_A11Y_SWEEP === '1';

test.skip(
  !RUN_A11Y_SWEEP,
  'nightly populated axe sweep (#2920 item 6); set RUN_A11Y_SWEEP=1 to run it',
);

test.beforeEach(({ browserName }) => {
  // The sweep is Chromium-pinned like visual.spec.ts: axe's target-size
  // measurement depends on the same pinned font metrics, and the nightly job
  // runs Chromium only. The all-browsers nightly job does not set the gate.
  test.skip(browserName !== 'chromium', 'populated axe sweep is Chromium-pinned (see #1479)');
});

const THEMES = ['light', 'dark'] as const;
type Theme = (typeof THEMES)[number];

const VIEWPORTS: ReadonlyArray<{
  label: string;
  viewport: { width: number; height: number };
}> = [
  // 1280x720 matches visual.spec.ts's baselines: the desktop layout a
  // maintainer's re-audit sweep used (#2907 reproduces here too).
  { label: 'desktop', viewport: { width: 1280, height: 720 } },
  // 375x800 is the phone width the round-2 re-audit used. `compactNav` is on
  // strictly below 600px, so the hamburger that carries the pending-share
  // badge only exists here (#2906).
  { label: 'phone', viewport: { width: 375, height: 800 } },
];

// The router's own table, parsed by the same script the route-coverage guard
// runs -- a new page is scanned the day it lands. `ROUTE_EXEMPTIONS` and
// `ADMIN_ROUTES` come from item 4's shared module, never re-listed here.
const ROUTES = loadRoutes();

expect(ROUTES.length, 'App.tsx parsed to no routes').toBeGreaterThan(20);

const SEEDED_ROUTES: readonly string[] = ROUTES.filter(
  (route) => !Object.hasOwn(ROUTE_EXEMPTIONS, route) && !ADMIN_ROUTES.includes(route),
);

async function openRoute(page: Page, path: string, theme: Theme): Promise<void> {
  await page.addInitScript((t) => localStorage.setItem('themePreference', t), theme);
  // Pin the app's own webfont and freeze the clock for the same reason
  // pseudoLocale.spec.ts does: target-size is a measured pixel value, so a
  // host's fallback font must not decide whether a 19.9px link fails 2.5.8.
  await pinRenderingEnvironment(page);
  await page.goto(path);
  await awaitPinFonts(page);
  await waitForLoading(page);
}

for (const { label, viewport } of VIEWPORTS) {
  test.describe(`Populated a11y sweep (${label} ${viewport.width}px)`, {
    // Every test scans explicitly; the automatic per-test scan from
    // `fixtures.ts` would re-scan the same page for zero added coverage.
    annotation: { type: SKIP_A11Y_SCAN, description: 'scans each route/theme explicitly' },
  }, () => {
    test.use({ viewport });

    for (const theme of THEMES) {
      for (const route of SEEDED_ROUTES) {
        test(`route ${route} (${theme}) has no critical or serious a11y violations`, async ({
          page,
          populatedSeed,
        }) => {
          await openRoute(page, routeFor(route, populatedSeed), theme);
          // Prove the seeded round-2 state rendered (pending share, occasion,
          // two-participant activity, anniversary share) instead of scanning
          // an empty/redirected page that would pass vacuously.
          await expectSeededRouteContent(page, route, populatedSeed);
          await assertNoBlockingA11yViolations(page);
        });
      }
    }
  });

  test.describe(`Populated a11y sweep admin routes (${label} ${viewport.width}px)`, {
    annotation: { type: SKIP_A11Y_SCAN, description: 'scans each route/theme explicitly' },
  }, () => {
    // Admin pages redirect a worker user to the dashboard, so they run as the
    // shared admin (same split as item 4). They need no seeded fixture.
    test.use({ viewport, sharedUser: true });

    for (const theme of THEMES) {
      for (const route of ADMIN_ROUTES) {
        test(`route ${route} (${theme}) has no critical or serious a11y violations`, async ({
          page,
        }) => {
          await openRoute(page, route, theme);
          await assertNoBlockingA11yViolations(page);
        });
      }
    }
  });
}
