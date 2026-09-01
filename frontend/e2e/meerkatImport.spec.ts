import { expect, test, waitForLoading } from './fixtures';

// Meerkat import assistant (issue #550). The full upload → pick user → review →
// confirm happy path is covered end to end by the Go integration test
// (backend/services/meerkat_import_session_test.go), which builds the fixture
// database with the Go-only meerkatfixture package. What this spec pins is the
// browser surface: the wizard opens from Data settings with a file picker, and
// a malformed / non-Meerkat file is rejected cleanly — no crash, no partial
// import, still on the connect step (issue #550's hostile-input requirement).
test.describe('Meerkat import assistant', () => {
  test('opens the wizard with a database-file picker', async ({ page }) => {
    await page.goto('/settings/data');
    await waitForLoading(page);

    await page.getByRole('button', { name: 'Import from Meerkat' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.locator('.MuiStepLabel-label')).toHaveText([
      'Connect',
      'Fetch',
      'Review',
      'Import',
      'Done',
    ]);
    await expect(dialog.getByRole('button', { name: 'Choose database file' })).toBeVisible();
    await expect(dialog.locator('#meerkat-file-input')).toHaveAttribute('type', 'file');
  });

  test('a non-Meerkat file is rejected cleanly and stays on connect', async ({ page }) => {
    await page.goto('/settings/data');
    await waitForLoading(page);

    await page.getByRole('button', { name: 'Import from Meerkat' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();

    await dialog.locator('#meerkat-file-input').setInputFiles({
      name: 'not-really.db',
      mimeType: 'application/octet-stream',
      buffer: Buffer.from('this is plainly not a sqlite database'),
    });

    await expect(dialog.getByRole('alert')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Choose database file' })).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Start import' })).toHaveCount(0);
  });
});
