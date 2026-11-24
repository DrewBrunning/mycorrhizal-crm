// e2e/populatedRoutes.ts
//
// The route inventory shared by the two #2920 populated sweeps. `loadRoutes()`
// in `scripts/check-e2e-routes.mjs` parses `src/App.tsx` (never a hand-kept
// list), so a new page is picked up the day it lands; this module holds the
// parts that are not derivable from the router: the redirect-only exemptions,
// the admin-only pages, how `:id` resolves to the seed, and the assertion that
// proves the seeded round-2 state actually rendered (an empty page would
// otherwise scan clean and hide a broken seed).
//
// `pseudoLocale.spec.ts` (item 4) and `a11ySweepPopulated.spec.ts` (item 6)
// both import it, so the two sweeps can never disagree about which routes are
// covered or why one is exempt.

import type { Page } from '@playwright/test';
import { expect } from './fixtures';
import type { PopulatedSeed } from './populatedSeed';

/**
 * Routes that cannot render a page of their own. Every entry is a pure
 * `<Navigate>` redirect: the target renders (and is scanned separately), while
 * this path immediately leaves. `pseudoLocale.spec.ts`'s inventory test
 * navigates each one and fails if it ever stops redirecting, so the exemption
 * cannot rot (#2920).
 */
export const ROUTE_EXEMPTIONS: Record<string, string> = {
  '/tags': 'Redirect-only compatibility path; /circles?tab=tags is scanned.',
  '/api-tokens':
    'Redirect-only compatibility path; the page moved under /settings, which is scanned.',
  '/register': 'Logged-in shell redirects registration to the dashboard (/), which is scanned.',
};

/**
 * Admin-only pages need the shared admin's session to render their own page
 * rather than redirecting a worker user back to the dashboard. They carry no
 * seeded-fixture dependency, so the sweeps run them in their own describe
 * below.
 */
export const ADMIN_ROUTES: readonly string[] = [
  '/audit',
  '/users',
  '/system-events',
  '/system-status',
];

/** Resolves `:id` route parameters to the seeded fixtures (#2920 item 4). */
export function routeFor(route: string, seed: PopulatedSeed): string {
  if (route === '/contacts/:id' || route === '/contacts/:id/prep') {
    return route.replace(':id', String(seed.contactId));
  }
  if (route === '/entities/:id') return route.replace(':id', seed.entityId);
  return route;
}

/**
 * Proves the seeded round-2 state rendered on a route, so a seed that silently
 * stopped creating the pending share / occasion / participants fails loudly
 * instead of turning the sweep vacuously green.
 */
export async function expectSeededRouteContent(
  page: Page,
  route: string,
  seed: PopulatedSeed,
): Promise<void> {
  if (route === '/contacts/:id') {
    await expect(page.getByText(seed.contactName).first()).toBeVisible();
    await expect(page.getByText(seed.occasionLabel).first()).toBeVisible();
  } else if (route === '/contacts/:id/prep') {
    await expect(page.getByText(seed.contactName).first()).toBeVisible();
    await expect(page.getByText(seed.activityTitle).first()).toBeVisible();
  } else if (route === '/activities') {
    await expect(page.getByText(seed.activityTitle).first()).toBeVisible();
  } else if (route === '/shared-with-me') {
    await expect(page.getByText(seed.acceptedShareLabel).first()).toBeVisible();
    await expect(page.getByText(seed.pendingShareLabel).first()).toBeVisible();
  } else if (route === '/shared-by-me') {
    await expect(page.getByText(seed.outgoingShareLabel).first()).toBeVisible();
  } else if (route === '/entities/:id') {
    await expect(page.getByText(seed.entityName).first()).toBeVisible();
  }
}
