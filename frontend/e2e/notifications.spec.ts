import { test, expect } from './fixtures';
import { withExclusiveUserSettings } from './fixtures';
import { API_BASE_URL } from './global-setup';

// N9 (docs/fork-plan/tickets/30-N9-notification-channels.md) — the
// notification channel settings surface plus the push-subscription API.
//
// The happy-path "delivers to a real ntfy/Gotify instance" is covered by the
// Go integration tests (services/notification_service_test.go), which run a
// live fake channel endpoint against the real migrated schema — an in-browser
// spec cannot reach a host-loopback fake server from the dockerised backend
// the e2e suite runs against. What this spec pins instead is the two things
// that matter to a user and only show up end to end:
//
//   1. configuring a channel persists across a reload, and
//   2. a misconfigured channel surfaces the failure with an explanation
//      ("Test failed: …") rather than failing silently — plus the client-side
//      private-address warning that stops a user typing a URL the server's
//      WEBHOOK_BLOCK_PRIVATE_URLS policy would silently block.
//
// Push subscription registration is exercised directly through the API (the
// browser Push API / service worker is unavailable under Playwright's test
// browser).
test.describe('Notification settings', () => {
  // /notifications/config is a singleton row on the shared TEST_USER, same
  // as /users/enabled-contact-fields and /users/date-format -- restore it in
  // a finally block (below) and hold the same cross-file lock those specs
  // use, so a leftover "ntfy enabled, garbage URL" state from this test
  // can't linger into (or race) anything else sharing the account. Gotify's
  // token is never echoed back by the API (see notification_controller.go's
  // notificationConfigResponse), so this spec never touches it and the
  // restore below doesn't need to recover one.
  test('configures ntfy, persists across reload, warns on private URLs, and reports test failures', async ({ page, request }) => {
    await withExclusiveUserSettings(async () => {
      const before = await request.get(`${API_BASE_URL}/notifications/config`);
      expect(before.ok()).toBeTruthy();
      const original = await before.json();

      try {
        await page.goto('/settings');

        // The Notifications card renders all three channels.
        const card = page.getByText('Choose how reminder notifications reach you');
        await expect(card).toBeVisible();
        await expect(page.getByText('Send reminders via ntfy')).toBeVisible();
        await expect(page.getByText('Send reminders via Gotify')).toBeVisible();
        await expect(page.getByText('Send reminders via browser push')).toBeVisible();

        // Configure ntfy: URL + topic + enable. A unique topic makes the
        // persistence assertion exact, and the toggle is flipped to a known
        // "checked" state rather than blindly clicked, so a leftover enabled state
        // from a previous (un-wiped) test DB cannot toggle it the wrong way.
        const topic = `e2e-alerts-${Date.now()}`;
        await page.getByLabel('ntfy server URL').fill('https://ntfy.example.com');
        await page.getByLabel('Topic').fill(topic);
        const ntfyToggle = page.getByLabel('Send reminders via ntfy');
        if (!(await ntfyToggle.isChecked())) {
          await ntfyToggle.click();
        }
        await expect(ntfyToggle).toBeChecked();
        await page.getByRole('button', { name: 'Save notification settings' }).click();

        // Save confirmation appears.
        await expect(page.getByText('Notification settings saved')).toBeVisible();

        // Reload: the saved values come back from the server.
        await page.reload();
        await expect(page.getByLabel('ntfy server URL')).toHaveValue('https://ntfy.example.com');
        await expect(page.getByLabel('Topic')).toHaveValue(topic);
        await expect(ntfyToggle).toBeChecked();

        // A private URL (as self-hosted ntfy servers usually are) gets a warning
        // that explains the WEBHOOK_BLOCK_PRIVATE_URLS policy before the user
        // ever saves it.
        await page.getByLabel('ntfy server URL').fill('http://localhost:8000');
        await expect(page.getByText(/WEBHOOK_BLOCK_PRIVATE_URLS/)).toBeVisible();

        // A misconfigured target must report the failure, not fail silently.
        await page.getByLabel('ntfy server URL').fill('http://127.0.0.1:1');
        const testButton = page.getByRole('button', { name: 'Send test notification' }).first();
        await expect(testButton).toBeEnabled();
        await testButton.click();
        await expect(page.getByText(/Test failed:/)).toBeVisible({ timeout: 15000 });
      } finally {
        // Restore exactly what was there before -- not just "disabled" -- so
        // a run against an already-configured account (or a re-run of this
        // test) leaves the shared account the way it found it.
        await request.put(`${API_BASE_URL}/notifications/config`, {
          data: {
            ntfy_url: original.ntfy_url ?? '',
            ntfy_topic: original.ntfy_topic ?? '',
            gotify_url: original.gotify_url ?? '',
            gotify_token: '',
            notify_ntfy: original.notify_ntfy ?? false,
            notify_gotify: original.notify_gotify ?? false,
            notify_push: original.notify_push ?? false,
          },
        }).catch(() => {});
      }
    });
  });

  test('push subscription registration is user-scoped CRUD via the API', async ({ request }) => {
    const endpoint = `https://push.example.com/e2e-${Date.now()}`;
    const create = await request.post(`${API_BASE_URL}/notifications/push-subscriptions`, {
      data: {
        endpoint,
        p256dh: 'BNNL5ZaTfK81qhXOx23-wewhigUeFb632jN6LvRWCFH1ubQr77FE_9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk',
        auth: 'zqbxT6JKstKSY9JKibZLSQ',
        device_label: 'E2E device',
      },
    });
    expect(create.status()).toBe(201);
    const created = await create.json();
    expect(created.endpoint).toBe(endpoint);

    // Listing shows it.
    const list = await request.get(`${API_BASE_URL}/notifications/push-subscriptions`);
    expect(list.status()).toBe(200);
    const listed = await list.json();
    const found = (listed.subscriptions as Array<{ id: number; endpoint: string }>).find(s => s.endpoint === endpoint);
    expect(found).toBeTruthy();

    // Delete cleans it up.
    const del = await request.delete(`${API_BASE_URL}/notifications/push-subscriptions/${created.id}`);
    expect(del.status()).toBe(200);

    const after = await request.get(`${API_BASE_URL}/notifications/push-subscriptions`);
    const afterBody = await after.json();
    expect((afterBody.subscriptions as Array<{ endpoint: string }>).some(s => s.endpoint === endpoint)).toBe(false);
  });
});
