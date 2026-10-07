import {
  createTestContact,
  deleteTestContact,
  expect,
  stableClick,
  test,
  waitForLoading,
} from './fixtures';

// Authenticated via the shared storageState (see playwright.config.ts).
// Each test runs against its own throwaway contact.
//
// waitForLoading + stableClick here for the same reason timeline.spec.ts needs
// them: the contact detail page mounts every panel eagerly and a section above
// Reminders (ConnectionsPanel) can defer its own fetch behind an
// IntersectionObserver, shifting the page mid-click. Without them the click on
// "Add Reminder" intermittently lands on the wrong element under parallel load.
test.describe('Reminders', () => {
  test('should create a reminder from a contact detail page', async ({ page, browserName }) => {
    // Issue #1479: on Firefox the scroll that precedes the click lands the
    // timeline's "View all" button / the jump-nav links under the sticky
    // ContactJumpNav, so axe's target-size check fails the post-test scan
    // (Chromium keeps them clear via scroll-padding-top, see ContactDetailPage).
    // A genuine Firefox layout difference, not a spec bug.
    test.fixme(
      browserName === 'firefox',
      'Firefox: sticky ContactJumpNav obscures the timeline View all button (axe target-size)',
    );
    const contact = await createTestContact(page.request);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: /add.*reminder/i }));
      await expect(page.getByRole('dialog')).toBeVisible();

      await page.getByRole('textbox', { name: /message/i }).fill('E2E Test Reminder');
      await page.getByRole('button', { name: /save|create/i }).click();

      // Dialog closes and the reminder shows up in the list.
      await expect(page.getByRole('dialog')).toBeHidden();
      await expect(page.getByText('E2E Test Reminder')).toBeVisible();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('should show reminder form fields', async ({ page, browserName }) => {
    // Issue #1479: on Firefox the scroll that precedes the click lands the
    // timeline's "View all" button / the jump-nav links under the sticky
    // ContactJumpNav, so axe's target-size check fails the post-test scan
    // (Chromium keeps them clear via scroll-padding-top, see ContactDetailPage).
    // A genuine Firefox layout difference, not a spec bug.
    test.fixme(
      browserName === 'firefox',
      'Firefox: sticky ContactJumpNav obscures the timeline View all button (axe target-size)',
    );
    const contact = await createTestContact(page.request);

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await stableClick(page.getByRole('button', { name: /add.*reminder/i }));
      await expect(page.getByRole('dialog')).toBeVisible();

      await expect(page.getByRole('textbox', { name: /message/i })).toBeVisible();
      await expect(page.getByLabel(/date/i).first()).toBeVisible();

      await page.keyboard.press('Escape');
      await expect(page.getByRole('dialog')).toBeHidden();
    } finally {
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
