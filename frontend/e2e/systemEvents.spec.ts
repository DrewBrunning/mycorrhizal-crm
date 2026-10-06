import { expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

// Issue #1481: the System Events page (issue #424) had no Playwright coverage.
// This drives a real operational event end to end: a webhook whose receiver
// answers a permanent 404 makes the delivery path record an `integration_failed`
// system event (severity error, component webhook), and the admin page must
// list it with the right severity, filter to it, show its detail, and grow past
// the first 100-row window when "Load more" is pressed.
//
// The page is admin-only, hence the shared (auto-admin) user. The event log is
// instance-wide and append-only, so the test creates its own events and asserts
// on those -- never on an absolute count.

test.use({ sharedUser: true });

// The 404 comes from the stack's own API (a route that does not exist), so no
// external network is needed. 4xx other than 408/429 is a permanent failure ->
// the event is raised on the first attempt.
const DEAD_RECEIVER = 'http://127.0.0.1:8080/api/v1/e2e-no-such-receiver';
const WINDOW = 100; // useSystemEvents DEFAULT_LIMIT

test.describe('System events', () => {
  test('a failed webhook delivery shows as an error event and the list loads more', async ({
    page,
  }) => {
    const created = await page.request.post(`${API_BASE_URL}/webhooks`, {
      data: {
        name: `e2e-dead-receiver-${Date.now()}`,
        url: DEAD_RECEIVER,
        events: ['contact.created'],
        is_active: true,
      },
    });
    expect(created.ok(), `webhook create: ${await created.text()}`).toBeTruthy();
    const webhookId = (await created.json()).webhook?.id ?? (await created.json()).id;

    try {
      const fire = async () => {
        const res = await page.request.post(`${API_BASE_URL}/webhooks/${webhookId}/test`);
        expect(res.ok()).toBeTruthy();
      };
      await fire();

      // Top the log up until the first window is full, so a second window exists.
      const listed = await page.request.get(`${API_BASE_URL}/admin/system-events?limit=${WINDOW}`);
      expect(listed.ok()).toBeTruthy();
      const have = (await listed.json()).system_events.length as number;
      for (let i = have; i < WINDOW + 1; i++) await fire();

      await page.goto('/system-events');
      await waitForLoading(page);
      await expect(page.getByRole('heading', { name: 'System Events', level: 1 })).toBeVisible();

      // --- first window: exactly 100 rows, newest first, with the Load more button ---
      // Scoped to the events table: the health/jobs panels above it render
      // their own <table>s.
      const rows = page
        .getByRole('table')
        .filter({ has: page.getByRole('columnheader', { name: 'Operation' }) })
        .locator('tbody tr');
      await expect(rows).toHaveCount(WINDOW);
      const newest = rows.first();
      await expect(newest.getByText('Error', { exact: true })).toBeVisible();
      await expect(newest.getByText('webhook', { exact: true })).toBeVisible();
      await expect(newest.getByText('Failure', { exact: true })).toBeVisible();

      // --- page 2: Load more grows the window past the first 100 ---
      await page.getByRole('button', { name: 'Load more' }).click();
      await expect.poll(() => rows.count()).toBeGreaterThan(WINDOW);

      // --- detail dialog shows the webhook failure, never the receiver URL ---
      await rows.first().getByRole('button', { name: 'Details' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByText('unexpected status 404')).toBeVisible();
      await expect(dialog).not.toContainText(DEAD_RECEIVER);
      await dialog.getByRole('button', { name: 'Close' }).click();

      // --- filters are server-side: severity=Info excludes the failure rows ---
      await page.getByLabel('Severity').click();
      await page.getByRole('option', { name: 'Info', exact: true }).click();
      await expect(rows.first()).toBeVisible();
      await expect(rows.getByText('Failure', { exact: true })).toHaveCount(0);
    } finally {
      await page.request.delete(`${API_BASE_URL}/webhooks/${webhookId}`);
    }
  });
});

test.describe('System status', () => {
  // /system-status is the sibling admin page to the event log (composite
  // health picture); it shares the admin-only API surface, so it rides here.
  test('renders the composite status for the admin', async ({ page }) => {
    await page.goto('/system-status');
    await expect(page.getByRole('heading', { name: 'System status', level: 1 })).toBeVisible();
    await expect(page.getByText('Overall status')).toBeVisible();
  });
});

test.describe('System events (non-admin)', () => {
  // The default per-worker user is a plain account, not the admin.
  test.use({ sharedUser: false });

  test('a non-admin cannot read the instance-wide log', async ({ page }) => {
    const res = await page.request.get(`${API_BASE_URL}/admin/system-events`);
    expect(res.status()).toBe(403);

    await page.goto('/system-events');
    await waitForLoading(page);
    // No route guard hides the page from a non-admin (the nav entry is hidden,
    // the API is the authority): what matters is that no event data renders.
    await expect(page.getByRole('columnheader', { name: 'Operation' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Details' })).toHaveCount(0);
  });
});
