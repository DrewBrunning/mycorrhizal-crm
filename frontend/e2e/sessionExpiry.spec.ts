import {
  createTestContact,
  deleteTestContact,
  expect,
  stableClick,
  test,
  waitForLoading,
} from './fixtures';
import { TEST_USER } from './global-setup';

// Issue #557: a 401 used to be a hard `window.location.href = '/login'` from
// inside the shared fetch wrapper -- unconditional, on any request. That
// tore down the whole React tree (and every unsaved field in it) with no
// prompt. These drive the real app against a real backend and prove the
// replacement: the tree survives, dirty state survives, and re-authenticating
// happens in place.
test.describe('Session expiry', () => {
  test('a 401 on Save keeps the note dialog and its content intact, and re-authenticating in place clears the prompt', async ({
    page,
  }) => {
    const contact = await createTestContact(page.request);
    const noteContent = `E2E session-expiry note ${Date.now()}`;

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: /add note/i }));
      // Scoped to the note dialog by its title -- once the re-auth or
      // discard-confirm dialog is also open, an unfiltered `dialog` locator
      // would match more than one element.
      const dialog = page.getByRole('dialog').filter({ hasText: 'Add Note' });
      await expect(dialog).toBeVisible({ timeout: 10000 });
      await dialog.getByLabel(/content/i).fill(noteContent);

      // Simulate the session having expired server-side: the next POST to
      // create the note comes back 401, as if the httpOnly auth cookie had
      // just expired. Every other request (login, etc.) passes through to
      // the real backend untouched.
      await page.route('**/api/v1/contacts/*/notes', async (route) => {
        if (route.request().method() === 'POST') {
          await route.fulfill({ status: 401, contentType: 'application/json', body: '{}' });
        } else {
          await route.continue();
        }
      });

      await dialog.getByRole('button', { name: /^save$/i }).click();

      // The blocking re-auth modal appears -- this is a mutating request the
      // user is actively waiting on.
      const reauthDialog = page.getByRole('dialog').filter({ hasText: /session expired/i });
      await expect(reauthDialog).toBeVisible({ timeout: 10000 });

      // The app never navigated away. (The note dialog is still there too,
      // but MUI marks a covered dialog aria-hidden while another one is on
      // top of it, which also makes it invisible to role-based locators --
      // the assertion after the re-auth dialog closes below covers that.)
      expect(page.url()).toContain(`/contacts/${contact.ID}`);

      // Sign back in without leaving the page.
      await reauthDialog.getByLabel(/username or email/i).fill(TEST_USER.username);
      await reauthDialog.getByLabel(/^password/i).fill(TEST_USER.password);
      await reauthDialog.getByRole('button', { name: /sign in/i }).click();

      await expect(reauthDialog).toBeHidden({ timeout: 10000 });

      // The note dialog is still open, with the content still there --
      // nothing remounted underneath the re-auth prompt.
      await expect(dialog).toBeVisible();
      await expect(dialog.getByLabel(/content/i)).toHaveValue(noteContent);

      // The interception only fails the very first Save attempt; a real
      // retry now goes to the real (re-authenticated) backend.
      await page.unroute('**/api/v1/contacts/*/notes');
      await dialog.getByRole('button', { name: /^save$/i }).click();
      await expect(dialog).toBeHidden();
      await expect(page.getByText(noteContent)).toBeVisible({ timeout: 10000 });
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('a 401 on a background load shows a passive banner, not a blocking prompt', async ({
    page,
  }) => {
    await page.route('**/api/v1/dashboard', async (route) => {
      await route.fulfill({ status: 401, contentType: 'application/json', body: '{}' });
    });

    await page.goto('/');

    // A dismissible, non-modal notice -- not the re-authentication dialog.
    await expect(page.getByText(/session has expired/i)).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('dialog')).toHaveCount(0);

    // Never navigated to /login, and the app shell (nav) is still there --
    // this is not the hard-redirect this ticket removed.
    expect(page.url()).not.toContain('/login');
    await expect(page.getByRole('navigation')).toBeVisible();
  });

  test('cancelling a dirty note asks for confirmation before discarding it', async ({ page }) => {
    const contact = await createTestContact(page.request);
    // Deliberately doesn't contain the word "discard" -- the confirm
    // dialog's own locator below filters on that word, and while both
    // dialogs are open a note body containing it would match too.
    const noteContent = `E2E dirty-form note ${Date.now()}`;

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: /add note/i }));
      // Scoped to the note dialog by its title -- once the re-auth or
      // discard-confirm dialog is also open, an unfiltered `dialog` locator
      // would match more than one element.
      const dialog = page.getByRole('dialog').filter({ hasText: 'Add Note' });
      await expect(dialog).toBeVisible({ timeout: 10000 });
      await dialog.getByLabel(/content/i).fill(noteContent);

      await dialog.getByRole('button', { name: /^cancel$/i }).click();

      const confirmDialog = page.getByRole('dialog').filter({ hasText: /discard/i });
      await expect(confirmDialog).toBeVisible();

      // Backing out keeps the dialog open with the typed content intact.
      await confirmDialog.getByRole('button', { name: /keep editing/i }).click();
      await expect(confirmDialog).toBeHidden();
      await expect(dialog).toBeVisible();
      await expect(dialog.getByLabel(/content/i)).toHaveValue(noteContent);

      // Confirming actually discards.
      await dialog.getByRole('button', { name: /^cancel$/i }).click();
      await expect(confirmDialog).toBeVisible();
      await confirmDialog.getByRole('button', { name: /^discard$/i }).click();
      await expect(dialog).toBeHidden();
      await expect(page.getByText(noteContent)).toHaveCount(0);
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
