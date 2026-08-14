import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact, waitForLoading, withExclusiveUserSettings } from './fixtures';
import { API_BASE_URL } from './global-setup';
import { DEFAULT_ENABLED_CONTACT_FIELDS } from '../src/contactFields';

// v0.4.1: the date format preference is built out (more than eu/us/iso), and
// every date surface -- including the anniversary rows, which used to render
// raw YYYY-MM-DD regardless of the selected format -- must honor it. This
// spec drives the real Settings dropdown (proving the new options exist and
// persist) and asserts a contact's birthday + wedding anniversary render in
// the chosen format on the contact page.
//
// The `anniversaries` array field is opt-in (not in DEFAULT_ENABLED_CONTACT_
// FIELDS, imported from source rather than hand-copied so this can't drift
// from the real default), so the spec enables it for the shared test user up
// front and restores the default set afterwards.
//
// This test and linkFieldTypeEditors.spec.ts's are the only two specs that
// mutate /users/enabled-contact-fields (a singleton row on the shared
// TEST_USER), so both wrap their whole body in withExclusiveUserSettings --
// see that helper's doc comment in fixtures.ts for why describe.configure's
// serial mode alone doesn't close this gap across files.
test.describe('Date format selection (v0.4.1)', () => {
  test('lists the built-out formats and renders birthdays + anniversaries in the chosen one', async ({ page, request }) => {
    await withExclusiveUserSettings(async () => {
      // Enable the (default-off) anniversary array field so the fixed
      // renderAnniversaries path is exercised, not just the birthday scalar.
      const enableResp = await request.patch(`${API_BASE_URL}/users/enabled-contact-fields`, {
        data: { fields: [...DEFAULT_ENABLED_CONTACT_FIELDS, 'anniversary', 'anniversaries'] },
      });
      expect(enableResp.ok()).toBeTruthy();

      const contact = await createTestContact(page.request, {
        firstname: 'E2EDateFmt',
        lastname: `V041${Date.now()}`,
        birthday: '1990-03-15',
        anniversary: '2015-06-01',
      });

      try {
        // Change the date format through the real Settings UI. The dropdown must
        // list the new built-out options.
        await page.goto('/settings');
        const formatSelect = page.getByLabel('Date Format');
        await expect(formatSelect).toBeVisible();
        await formatSelect.click();

        await expect(page.getByRole('option', { name: 'Canada (DD/MM/YYYY)' })).toBeVisible();
        await expect(page.getByRole('option', { name: 'European with hyphens (DD-MM-YYYY)' })).toBeVisible();
        await expect(page.getByRole('option', { name: 'US abbreviated (MMM D, YYYY)' })).toBeVisible();
        await expect(page.getByRole('option', { name: 'US full (MMMM D, YYYY)' })).toBeVisible();

        await page.getByRole('option', { name: 'Canada (DD/MM/YYYY)' }).click();

        // Backend persisted the new format (the e2e can see it via the API).
        const me = await request.get(`${API_BASE_URL}/users/me`);
        const meBody = await me.json();
        expect(meBody.date_format).toBe('ca');

        // The contact page renders birthday + wedding anniversary in DD/MM/YYYY.
        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);
        // The birthday EditableField shows the date + age; the anniversary row
        // shows the date + kind label. Assert on the row forms rather than the
        // bare date text, which legitimately appears in both.
        await expect(page.getByText('15/03/1990 (Birthday)')).toBeVisible();
        await expect(page.getByText(/01\/06\/2015 \(Wedding anniversary\)/)).toBeVisible();
        // The anniversary row itself must not fall back to raw ISO (the bug).
        // (The backend mirrors a wedding anniversary into a "Married" LifeEvent
        // whose timeline row legitimately renders ISO via partialDateDisplay, so
        // scope the negative check to the anniversary row, not the whole page.)
        await expect(page.getByText('2015-06-01 (Wedding anniversary)')).not.toBeAttached();

        // Switching format re-renders: pick the full-month US format and confirm
        // the dates follow.
        await page.goto('/settings');
        await page.getByLabel('Date Format').click();
        await page.getByRole('option', { name: 'US full (MMMM D, YYYY)' }).click();

        await page.goto(`/contacts/${contact.ID}`);
        await waitForLoading(page);
        await expect(page.getByText('March 15, 1990 (Birthday)')).toBeVisible();
        await expect(page.getByText(/June 1, 2015 \(Wedding anniversary\)/)).toBeVisible();
      } finally {
        await deleteTestContact(page.request, contact.ID);
        // Restore the shared test user's defaults so other specs aren't affected.
        await request.patch(`${API_BASE_URL}/users/date-format`, {
          data: { date_format: 'eu' },
        }).catch(() => {});
        await request.patch(`${API_BASE_URL}/users/enabled-contact-fields`, {
          data: { fields: DEFAULT_ENABLED_CONTACT_FIELDS },
        }).catch(() => {});
      }
    });
  });
});
