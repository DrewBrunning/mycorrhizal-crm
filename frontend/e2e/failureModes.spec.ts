import type { Page, Route } from '@playwright/test';
import {
  createTestContact,
  deleteTestContact,
  expect,
  stableClick,
  test,
  waitForLoading,
} from './fixtures';

// Issue #1478: the app's error UX was verified almost entirely in vitest with
// a mocked `fetch`; the real browser + real bundle almost never saw the
// backend fail. Each test here makes one real request fail (route-stubbed, so
// it is fast and deterministic, with the backend's real error-envelope shape
// from `backend/errors/errors.go`) or takes the network away, and asserts:
//
//   * the failure is visible (an alert, not a blank page or a lie such as
//     "Contact not found") and retryable;
//   * server internals in `error.details` do not leak into the DOM;
//   * the page threw no uncaught exception and logged no `console.error`
//     beyond the app's own expected `[operation] Error:` line and the
//     browser's own "Failed to load resource" network line;
//   * the automatic axe scan from fixtures.ts (it runs over whatever the
//     test leaves on screen) passes on the *error state* -- each test ends
//     there deliberately, or recovers and ends on the healthy state.
//
// 412 Precondition Failed is deliberately absent: the web client never sends
// `If-Match` today (ADR 0008, opt-in enforcement), so it is a missing feature
// rather than a missing test -- see the issue's grooming note.
test.describe('Failure modes', { tag: '@failure-modes' }, () => {
  const LEAK_SENTINEL = 'INTERNAL-STACK-TRACE-SENTINEL';

  /** Collects uncaught exceptions and unexpected console.error output. */
  function watchPage(page: Page) {
    const pageErrors: string[] = [];
    const consoleErrors: string[] = [];
    page.on('pageerror', (e) => pageErrors.push(String(e)));
    page.on('console', (m) => {
      if (m.type() === 'error') consoleErrors.push(m.text());
    });
    // Expected: the browser's own network-failure line, and the app's
    // structured `[operation] Error:` / `Error fetching ...:` log. Anything
    // else is a real, unexpected console.error.
    const EXPECTED = /^Failed to load resource|^\[[^\]]+\] Error:|^Error fetching/;
    return {
      assertClean() {
        expect(pageErrors, 'uncaught page errors').toEqual([]);
        expect(
          consoleErrors.filter((t) => !EXPECTED.test(t)),
          'unexpected console.error output',
        ).toEqual([]);
      },
    };
  }

  /** The backend's error envelope (errors.go) with a leaky `details` map. */
  function errorBody(code: string, message: string, details?: Record<string, string>) {
    return JSON.stringify({
      error: { code, message, ...(details ? { details } : {}) },
      request_id: 'req-e2e-1478',
      timestamp: '2026-10-06T00:00:00Z',
    });
  }

  function failWith(status: number, code: string, message: string, headers = {}) {
    return (route: Route) =>
      route.fulfill({
        status,
        contentType: 'application/json',
        headers,
        body: errorBody(code, message, { internal: LEAK_SENTINEL }),
      });
  }

  /**
   * Matches one exact API path (the list vs the detail endpoint). Memoised per
   * path so `page.unroute(apiPath(p))` gets the SAME function the route was
   * registered with -- Playwright unroutes by reference, and a fresh closure
   * silently removes nothing.
   */
  const matchers = new Map<string, (url: URL) => boolean>();
  const apiPath = (path: string) => {
    let m = matchers.get(path);
    if (!m) {
      m = (url: URL) => url.pathname === `/api/v1${path}`;
      matchers.set(path, m);
    }
    return m;
  };

  async function expectNoLeak(page: Page) {
    await expect(page.locator('body')).not.toContainText(LEAK_SENTINEL);
  }

  test('a 503 on the dashboard shows a retryable error with the request ref, no internals', async ({
    page,
  }) => {
    const watch = watchPage(page);
    await page.route(apiPath('/dashboard'), failWith(503, 'INTERNAL_ERROR', 'Service unavailable'));

    await page.goto('/');
    const alert = page.getByRole('alert').filter({ hasText: /service unavailable/i });
    await expect(alert).toBeVisible();
    // 5xx carries the request id so a user's report can be found in the logs.
    await expect(alert).toContainText('req-e2e-1478');
    await expectNoLeak(page);
    watch.assertClean();

    // Retry goes to the (recovered) backend and renders the real dashboard.
    await page.unroute(apiPath('/dashboard'));
    await alert.getByRole('button', { name: /try again/i }).click();
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
    await expect(alert).toBeHidden();
  });

  test('a 503 on the contacts list shows a retryable error, not a silently empty list', async ({
    page,
  }) => {
    const watch = watchPage(page);
    await page.route(
      apiPath('/contacts'),
      failWith(503, 'INTERNAL_ERROR', 'Contacts are unavailable'),
    );

    await page.goto('/contacts');
    const alert = page.getByRole('alert').filter({ hasText: /contacts are unavailable/i });
    await expect(alert).toBeVisible();
    await expect(page.getByTestId('contact-card')).toHaveCount(0);
    await expectNoLeak(page);
    watch.assertClean();

    await page.unroute(apiPath('/contacts'));
    await alert.getByRole('button', { name: /try again/i }).click();
    await expect(page.getByTestId('contact-card').first()).toBeVisible();
    await expect(alert).toBeHidden();
  });

  test('a 503 on the contact detail shows a retryable error, not "Contact not found"', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request);
    const watch = watchPage(page);
    const detail = (url: URL) => url.pathname === `/api/v1/contacts/${contact.ID}`;
    try {
      await page.route(detail, failWith(503, 'INTERNAL_ERROR', 'Detail is unavailable'));

      await page.goto(`/contacts/${contact.ID}`);
      const alert = page.getByRole('alert').filter({ hasText: /detail is unavailable/i });
      await expect(alert).toBeVisible();
      await expect(page.getByText(/contact not found/i)).toHaveCount(0);
      await expectNoLeak(page);
      watch.assertClean();

      await page.unroute(detail);
      await alert.getByRole('button', { name: /try again/i }).click();
      await waitForLoading(page);
      await expect(
        page.getByRole('heading', { name: new RegExp(contact.firstname) }),
      ).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('a genuine 404 on the contact detail still says "Contact not found" (no retry offered)', async ({
    page,
  }) => {
    const watch = watchPage(page);
    await page.goto('/contacts/987654321');
    await expect(page.getByText(/contact not found/i)).toBeVisible();
    await expect(page.getByRole('button', { name: /try again/i })).toHaveCount(0);
    watch.assertClean();
  });

  test('the network dropping mid-session surfaces an error, and the page recovers once back online', async ({
    page,
    context,
  }) => {
    const watch = watchPage(page);
    await page.goto('/contacts');
    await expect(page.getByTestId('contact-card').first()).toBeVisible();

    await context.setOffline(true);
    // Client-side navigation: the shell is already loaded, only the dashboard
    // data fetch has to cross the (now dead) network.
    await page
      .getByRole('link', { name: /^dashboard$/i })
      .first()
      .click();
    const alert = page.getByRole('alert').filter({ hasText: /fetch|network|connection/i });
    await expect(alert).toBeVisible();
    watch.assertClean();

    await context.setOffline(false);
    await alert.getByRole('button', { name: /try again/i }).click();
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
    await expect(alert).toBeHidden();
  });

  test('a 429 from the rate limiter shows its message and the page recovers on retry', async ({
    page,
  }) => {
    const watch = watchPage(page);
    // The shape `ErrRateLimitExceeded()` produces, with the limiter's Retry-After.
    await page.route(apiPath('/dashboard'), (route) =>
      route.fulfill({
        status: 429,
        contentType: 'application/json',
        headers: { 'Retry-After': '1' },
        body: errorBody('RATE_LIMIT_EXCEEDED', 'Rate limit exceeded. Please try again later.', {
          internal: LEAK_SENTINEL,
        }),
      }),
    );

    await page.goto('/');
    const alert = page.getByRole('alert').filter({ hasText: /rate limit exceeded/i });
    await expect(alert).toBeVisible();
    await expectNoLeak(page);
    watch.assertClean();

    await page.unroute(apiPath('/dashboard'));
    await alert.getByRole('button', { name: /try again/i }).click();
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible();
  });

  test('a 401 mid-save keeps the add-contact draft, re-auths in place (no redirect), and the retry saves it', async ({
    page,
    workerUser,
  }) => {
    const watch = watchPage(page);
    const firstname = `E2E-401-${Date.now()}`;
    let createdId: number | undefined;
    try {
      await page.goto('/contacts');
      await waitForLoading(page);
      await stableClick(page.getByRole('button', { name: /add contact/i }));
      const dialog = page.getByRole('dialog').filter({ hasText: /add new contact/i });
      await expect(dialog).toBeVisible();
      await dialog.getByLabel(/first name/i).fill(firstname);
      await dialog.getByLabel(/last name/i).fill('Draft');

      const create = (url: URL) => url.pathname === '/api/v1/contacts';
      await page.route(create, (route) =>
        route.request().method() === 'POST'
          ? route.fulfill({ status: 401, contentType: 'application/json', body: '{}' })
          : route.fallback(),
      );
      await dialog.getByRole('button', { name: /^create$/i }).click();

      const reauth = page.getByRole('dialog', { name: /^session expired$/i });
      await expect(reauth).toBeVisible();
      // In place: no hard navigation to /login that would discard the draft.
      expect(new URL(page.url()).pathname).toBe('/contacts');

      await reauth.getByLabel(/username or email/i).fill(workerUser.username);
      await reauth.getByLabel(/^password/i).fill(workerUser.password);
      await reauth.getByRole('button', { name: /sign in/i }).click();
      await expect(reauth).toBeHidden();

      await expect(dialog).toBeVisible();
      await expect(dialog.getByLabel(/first name/i)).toHaveValue(firstname);
      await expect(dialog.getByLabel(/last name/i)).toHaveValue('Draft');

      await page.unroute(create);
      const created = page.waitForResponse(
        (r) => r.url().endsWith('/api/v1/contacts') && r.request().method() === 'POST',
      );
      await dialog.getByRole('button', { name: /^create$/i }).click();
      const body = (await (await created).json()) as { contact: { id: number } };
      createdId = body.contact.id;
      await expect(dialog).toBeHidden();
      watch.assertClean();
    } finally {
      if (createdId) await deleteTestContact(page.request, createdId);
    }
  });

  test('a field-level validation error lands on the field it names, and the draft survives', async ({
    page,
  }) => {
    const watch = watchPage(page);
    // The real response for a create the server rejects on `card.name`
    // (contact_controller.go) -- ErrValidation is a 400 VALIDATION_ERROR whose
    // `details` are keyed by JSON path.
    await page.route(apiPath('/contacts'), (route) =>
      route.request().method() === 'POST'
        ? route.fulfill({
            status: 400,
            contentType: 'application/json',
            body: errorBody('VALIDATION_ERROR', 'Request validation failed', {
              'card.name': 'Name is not acceptable to the server',
            }),
          })
        : route.fallback(),
    );

    await page.goto('/contacts');
    await waitForLoading(page);
    await stableClick(page.getByRole('button', { name: /add contact/i }));
    const dialog = page.getByRole('dialog').filter({ hasText: /add new contact/i });
    await dialog.getByLabel(/first name/i).fill('Rejected');
    await dialog.getByLabel(/last name/i).fill('ByServer');
    await dialog.getByRole('button', { name: /^create$/i }).click();

    const field = dialog.getByLabel(/first name/i);
    await expect(field).toHaveAttribute('aria-invalid', 'true');
    await expect(dialog.getByText('Name is not acceptable to the server')).toBeVisible();
    await expect(field).toHaveValue('Rejected');
    await expect(dialog.getByLabel(/last name/i)).toHaveValue('ByServer');
    // Editing the field clears the server-side error.
    await field.fill('Rejected again');
    await expect(field).not.toHaveAttribute('aria-invalid', 'true');
    watch.assertClean();
  });

  test('a response slower than 5s shows loading skeletons, then the content (no spinner-forever)', async ({
    page,
  }) => {
    const watch = watchPage(page);
    await page.route(apiPath('/dashboard'), async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 6000));
      await route.fallback();
    });

    await page.goto('/');
    // Well into the delay: still the skeleton, and not an error state.
    await page.waitForTimeout(2000);
    await expect(page.locator('.MuiSkeleton-root').first()).toBeVisible();
    await expect(page.getByRole('alert')).toHaveCount(0);

    // Past the delay: the skeleton is gone and the real dashboard is there.
    await expect(page.getByRole('heading', { name: /dashboard/i })).toBeVisible({
      timeout: 15000,
    });
    await waitForLoading(page);
    watch.assertClean();
  });
});
