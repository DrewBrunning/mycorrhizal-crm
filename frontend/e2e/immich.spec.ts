import { test, expect } from './fixtures';
import { waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

test.describe('Immich integration', () => {
  test.beforeEach(async ({ request }) => {
    await request.delete(`${API_BASE_URL}/immich/config`);
  });

  test.afterEach(async ({ request }) => {
    await request.delete(`${API_BASE_URL}/immich/config`);
  });

  // ── Settings card ──────────────────────────────────────────────

  test('renders the connect form when nothing is configured', async ({ page }) => {
    await page.goto('/settings');
    await waitForLoading(page);

    await expect(page.getByText('Immich').first()).toBeVisible();
    await expect(page.getByLabel(/Base URL$/i)).toBeVisible();
    await expect(page.getByLabel(/API Key/i)).toBeVisible();
    await expect(page.getByRole('button', { name: /Save connection/i })).toBeVisible();
  });

  test('rejects a non-http base URL client-side with a readable message', async ({
    page,
  }) => {
    await page.goto('/settings');
    await waitForLoading(page);

    await page.getByLabel(/Base URL$/i).fill('ftp://immich.example');
    await page.getByLabel(/API Key/i).fill('test-key-123');
    await page.getByRole('button', { name: /Save connection/i }).click();

    await expect(page.getByText(/http:\/\/ or https:\/\//i)).toBeVisible({ timeout: 5000 });
  });

  test('saves configuration and persists across reload', async ({ page }) => {
    await page.goto('/settings');
    await waitForLoading(page);

    await page.getByLabel(/Base URL$/i).fill('https://immich.example');
    await page.getByLabel(/API Key/i).fill('test-api-key-abc');
    await page.getByRole('button', { name: /Save connection/i }).click();

    await expect(page.getByRole('button', { name: /Remove connection/i })).toBeVisible({
      timeout: 5000,
    });

    await page.reload();
    await waitForLoading(page);

    // The Immich config is fetched asynchronously on mount — wait for the
    // field to repopulate rather than asserting immediately.
    await expect(page.getByLabel(/Base URL$/i)).toHaveValue('https://immich.example', {
      timeout: 10000,
    });
    await expect(page.getByRole('button', { name: /Remove connection/i })).toBeVisible();
    await expect(
      page.getByLabel(/Automatically sync photo appearances/i),
    ).toBeVisible();
  });

  test('shows sync toggle and remove button after configuration', async ({
    page,
    request,
  }) => {
    await request.put(`${API_BASE_URL}/immich/config`, {
      data: { base_url: 'https://immich.example', api_key: 'key' },
    });

    await page.goto('/settings');
    await waitForLoading(page);

    await expect(page.getByRole('button', { name: /Remove connection/i })).toBeVisible();
    await expect(
      page.getByLabel(/Automatically sync photo appearances/i),
    ).toBeVisible();
  });

  test('test connection reports a diagnosed failure when Immich is unreachable', async ({
    page,
  }) => {
    await page.goto('/settings');
    await waitForLoading(page);

    await page.getByLabel(/Base URL$/i).fill('https://127.0.0.1:1');
    await page.getByLabel(/API Key/i).fill('any-key');
    await page.getByRole('button', { name: /Save connection/i }).click();
    await expect(page.getByRole('button', { name: /Remove connection/i })).toBeVisible({
      timeout: 5000,
    });

    await page.getByRole('button', { name: /Test connection/i }).click();

    await expect(
      page.getByText(/Could not reach|unreachable|not be reached/i),
    ).toBeVisible({ timeout: 15000 });
  });

  test('removes the connection and returns to the connect form', async ({
    page,
    request,
  }) => {
    await request.put(`${API_BASE_URL}/immich/config`, {
      data: { base_url: 'https://immich.example', api_key: 'key' },
    });

    await page.goto('/settings');
    await waitForLoading(page);

    page.once('dialog', (dialog) => dialog.accept());

    await page.getByRole('button', { name: /Remove connection/i }).click();

    await expect(page.getByLabel(/API Key/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole('button', { name: /Save connection/i })).toBeVisible();
  });

  // ── Contact-page Immich surface ─────────────────────────────────

  test('shows Immich link affordance on contact page when configured', async ({
    page,
    request,
  }) => {
    await request.put(`${API_BASE_URL}/immich/config`, {
      data: { base_url: 'https://immich.example', api_key: 'key' },
    });

    const contacts = await request.get(`${API_BASE_URL}/contacts?limit=1`);
    const { contacts: contactList } = await contacts.json();
    expect(contactList.length).toBeGreaterThan(0);
    const first = contactList[0];

    await page.goto(`/contacts/${first.ID}`);
    await waitForLoading(page);

    // The Immich config fetch runs in a useEffect on the contact page —
    // wait for the "Link a person" button to appear after the async fetch.
    await expect(
      page.getByRole('button', { name: /Link a person/i }),
    ).toBeVisible({ timeout: 10000 });
  });

  test('shows no Immich prompt on contact page when not configured', async ({
    page,
    request,
  }) => {
    const contacts = await request.get(`${API_BASE_URL}/contacts?limit=1`);
    const { contacts: contactList } = await contacts.json();
    expect(contactList.length).toBeGreaterThan(0);
    const first = contactList[0];

    await page.goto(`/contacts/${first.ID}`);
    await waitForLoading(page);

    // When no Immich config exists, the External Link panel renders
    // without the Immich-specific section — the "Link a person" button
    // must not appear.
    await expect(
      page.getByRole('button', { name: /Link a person/i }),
    ).not.toBeVisible({ timeout: 3000 });
  });

  // ── URL allowlist ───────────────────────────────────────────────

  test('API rejects a non-http scheme on immich/config PUT', async ({ request }) => {
    const res = await request.put(`${API_BASE_URL}/immich/config`, {
      data: { base_url: 'ftp://immich.example', api_key: 'key' },
    });
    expect(res.status(), await res.text()).toBe(400);
  });

  // ── Error states ────────────────────────────────────────────────

  test('contact page does not crash when Immich is unreachable', async ({
    page,
    request,
  }) => {
    await request.put(`${API_BASE_URL}/immich/config`, {
      data: { base_url: 'https://127.0.0.1:1', api_key: 'key' },
    });

    const contacts = await request.get(`${API_BASE_URL}/contacts?limit=1`);
    const { contacts: contactList } = await contacts.json();
    expect(contactList.length).toBeGreaterThan(0);
    const first = contactList[0];

    await page.goto(`/contacts/${first.ID}`);
    await waitForLoading(page);

    // The page must render the contact name heading even when the Immich
    // summary fetch fails (T15/T16 trap: an unavailable instance must
    // degrade, not crash the entire contact page).
    await expect(
      page.getByRole('heading', { name: new RegExp(first.firstname) }),
    ).toBeVisible({ timeout: 10000 });
  });

  test('browsing people surfaces an error without crashing when Immich is unreachable', async ({
    page,
  }) => {
    await page.goto('/settings');
    await waitForLoading(page);

    await page.getByLabel(/Base URL$/i).fill('https://127.0.0.1:1');
    await page.getByLabel(/API Key/i).fill('any-key');
    await page.getByRole('button', { name: /Save connection/i }).click();
    await expect(page.getByRole('button', { name: /Remove connection/i })).toBeVisible({
      timeout: 5000,
    });

    const contacts = await page.request.get(`${API_BASE_URL}/contacts?limit=1`);
    const { contacts: contactList } = await contacts.json();
    expect(contactList.length).toBeGreaterThan(0);
    const first = contactList[0];

    await page.goto(`/contacts/${first.ID}`);
    await waitForLoading(page);

    // Click "Link a person" — the config exists so the button renders.
    // The person picker fires ListPeople on open, which fails because
    // the Immich server is unreachable. The error must be surfaced as
    // an Alert inside the dialog, not a page crash.
    const linkButton = page.getByRole('button', { name: /Link a person/i });
    await expect(linkButton).toBeVisible({ timeout: 10000 });
    await linkButton.click();

    await expect(
      page.getByText(/could not reach|not be reached|is the instance up/i),
    ).toBeVisible({ timeout: 15000 });
  });
});
