import { expect, test, waitForLoading } from './fixtures';

// Monica import assistant (issue #549). The full connect → fetch → review →
// confirm happy path is covered end-to-end by the Go integration test
// (backend/services/monica_import_session_test.go) against an httptest Monica,
// because the e2e backend runs in a container that cannot reach a mock server
// on the test host. What this spec pins is the browser surface: the wizard
// opens from Data settings, the connect form is what issue #549 asks for
// (instance URL + API token entered in the UI, no export file), and a bad
// connection surfaces inline without leaving the connect step or crashing.
test.describe('Monica import assistant', () => {
  test('opens the wizard and shows the connect form', async ({ page }) => {
    await page.goto('/settings/data');
    await waitForLoading(page);

    await page.getByRole('button', { name: 'Import from Monica' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    // The five shared source-import steps.
    for (const label of ['Connect', 'Fetch', 'Review', 'Import', 'Done']) {
      await expect(dialog.getByText(label, { exact: true })).toBeVisible();
    }
    // Credentials are entered in the UI (no manual export step).
    await expect(dialog.getByLabel('Monica address')).toBeVisible();
    await expect(dialog.getByLabel('API token')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Connect' })).toBeVisible();
  });

  test('a bad connection is reported inline and stays on the connect step', async ({ page }) => {
    await page.goto('/settings/data');
    await waitForLoading(page);

    await page.getByRole('button', { name: 'Import from Monica' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.getByLabel('Monica address').fill('htp://not-a-real-scheme');
    await dialog.getByLabel('API token').fill('irrelevant-token');
    await dialog.getByRole('button', { name: 'Connect' }).click();

    // An error alert appears; the wizard has not advanced past connect.
    await expect(dialog.getByRole('alert')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Connect' })).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Start import' })).toHaveCount(0);
  });
});
