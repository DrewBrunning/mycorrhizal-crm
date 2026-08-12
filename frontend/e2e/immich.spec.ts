import { test, expect } from './fixtures';
import { waitForLoading, createTestContact, deleteTestContact } from './fixtures';
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

    // After reload, the Immich config is fetched asynchronously. The
    // "Remove connection" button appearing proves the config loaded
    // and persisted.  (Sync toggle + test connection button are gated
    // on has_api_key which arrives in the same response — tested
    // separately in the API-seeded config test.)
    await expect(page.getByRole('button', { name: /Remove connection/i })).toBeVisible({
      timeout: 10000,
    });
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
      page.getByText(/Automatically sync photo appearances/i),
    ).toBeVisible();
  });

  test('test connection button is present and clickable when configured', async ({
    page,
    request,
  }) => {
    await request.put(`${API_BASE_URL}/immich/config`, {
      data: { base_url: 'https://immich.example', api_key: 'any-key' },
    });

    await page.goto('/settings');
    await waitForLoading(page);

    const testBtn = page.getByRole('button', { name: /Test connection/i });
    await expect(testBtn).toBeVisible({ timeout: 5000 });
    await expect(testBtn).toBeEnabled();

    // Clicking the button must not crash the page.  (The backend
    // integration tests cover the diagnostic response shapes; the
    // e2e test verifies the UI wiring works without errors.)
    await testBtn.click();
    await waitForLoading(page);
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

    const contact = await createTestContact(request, {
      firstname: 'ImmichTest',
      lastname: 'LinkAffordance',
    });

    await page.goto(`/contacts/${contact.ID}`);
    await waitForLoading(page);

    // The Immich config fetch runs in a useEffect on the contact page —
    // wait for the "Link a person" button to appear after the async fetch.
    await expect(
      page.getByRole('button', { name: /Link a person/i }),
    ).toBeVisible({ timeout: 10000 });

    await deleteTestContact(request, contact.ID);
  });

  // T83: the contact-page Immich summary (`FetchImmichPersonSummary`, the
  // limit-1 RecentAssets caller) is what renders this linked-person row. The
  // e2e environment has no reachable Immich, so the live fetch degrades to
  // the identity's cached metadata — which is the meaningful assertion here:
  // a *linked* person's contact page renders its Immich surface from that
  // cache without crashing, the exact path T83's request bounding serves.
  test('linked person renders on the contact page from cached metadata', async ({
    page,
    request,
  }) => {
    await request.put(`${API_BASE_URL}/immich/config`, {
      data: { base_url: 'https://immich.example', api_key: 'key' },
    });

    const contact = await createTestContact(request, {
      firstname: 'ImmichTest',
      lastname: 'LinkedSummary',
    });

    // Link without browsing: the link endpoint only persists the
    // ExternalIdentity, it never calls Immich (which is unreachable here).
    const link = await request.post(
      `${API_BASE_URL}/immich/contacts/${contact.uid}/link`,
      { data: { person_id: 'person-linked', person_name: 'Linked Person Name' } }
    );
    expect(link.ok(), await link.text()).toBeTruthy();

    await page.goto(`/contacts/${contact.ID}`);
    await waitForLoading(page);

    // The linked person's name comes from the summary's cached metadata (the
    // Immich instance is unreachable, so the live fetch degrades); the
    // "Immich" chip marks the row. The page must render both without errors.
    await expect(page.getByText('Linked Person Name')).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText('Immich', { exact: true }).first()).toBeVisible();

    await deleteTestContact(request, contact.ID);
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
      data: { base_url: 'https://immich.example', api_key: 'key' },
    });

    const contact = await createTestContact(request, {
      firstname: 'ImmichTest',
      lastname: 'Degradation',
    });

    await page.goto(`/contacts/${contact.ID}`);
    await waitForLoading(page);

    // The page must render the contact name heading even when the Immich
    // summary fetch fails (T15/T16 trap: an unavailable instance must
    // degrade, not crash the entire contact page).
    await expect(
      page.getByRole('heading', { name: new RegExp(contact.firstname) }),
    ).toBeVisible({ timeout: 10000 });

    await deleteTestContact(request, contact.ID);
  });

  test('browsing people surfaces an error without crashing when Immich is unreachable', async ({
    page,
    request,
  }) => {
    await page.goto('/settings');
    await waitForLoading(page);

    await page.getByLabel(/Base URL$/i).fill('http://127.0.0.1:54321');
    await page.getByLabel(/API Key/i).fill('any-key');
    await page.getByRole('button', { name: /Save connection/i }).click();
    await expect(page.getByRole('button', { name: /Remove connection/i })).toBeVisible({
      timeout: 5000,
    });

    const contact = await createTestContact(request, {
      firstname: 'ImmichTest',
      lastname: 'BrowseError',
    });

    await page.goto(`/contacts/${contact.ID}`);
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

    await deleteTestContact(request, contact.ID);
  });
});
