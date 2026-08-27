import { createTestContact, deleteTestContact, expect, test, waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

// Issue #173: favorite contacts.
//
// Authenticated via the shared storageState (see playwright.config.ts).
// Each test creates a throwaway contact, exercises the favorite flow against
// it, and deletes it in `finally` so runs stay idempotent. The favorite flag
// rides on a soft-deleted contact harmlessly: soft-deleted rows are excluded
// from every list and from the dashboard favorites block.

test.describe('Favorites', () => {
  test('toggles a favorite from the contact detail page', async ({ page, request }) => {
    const contact = await createTestContact(page.request, { lastname: 'FavToggle' });
    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);
      await expect(
        page.getByRole('heading', { name: `${contact.firstname} FavToggle` }),
      ).toBeVisible();

      // Non-favorite: the header star reads "Mark as favorite".
      const mark = page.getByRole('button', { name: 'Mark as favorite' });
      await expect(mark).toBeVisible();
      await mark.click();

      // Favorited: the star reads "Unmark as favorite".
      const unmark = page.getByRole('button', { name: 'Unmark as favorite' });
      await expect(unmark).toBeVisible();

      // The flag persisted server-side: the API must now report it. The header
      // flips optimistically (ContactDetailPage.handleToggleFavorite sets state
      // before awaiting POST /contacts/:id/favorite), so poll the API until the
      // write lands rather than reading once against that race.
      await expect
        .poll(async () => {
          const record = await request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
          return (await record.json()).is_favorite as boolean;
        })
        .toBe(true);

      // Toggle back off and confirm.
      await unmark.click();
      await expect(page.getByRole('button', { name: 'Mark as favorite' })).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('the favorites-only filter narrows the contacts list', async ({ page, request }) => {
    // The Contacts page defaults to the contactable-only filter (T103), so
    // both contacts must carry an email or they would be hidden before the
    // favorites switch even matters.
    const favorite = await createTestContact(page.request, {
      lastname: 'FavFilter',
      email: 'favfilter@example.com',
    });
    const plain = await createTestContact(page.request, {
      lastname: 'FavPlain',
      email: 'favplain@example.com',
    });
    try {
      // Favorite one contact via the API (the endpoint's wire shape is
      // covered by the detail-page test above).
      const fav = await request.post(`${API_BASE_URL}/contacts/${favorite.ID}/favorite`);
      expect(fav.ok()).toBeTruthy();

      await page.goto('/contacts');
      await waitForLoading(page);

      // Off by default: both contacts are listed (rows render "firstname lastname").
      const favFullName = `${favorite.firstname} FavFilter`;
      const plainFullName = `${plain.firstname} FavPlain`;
      await expect(page.getByText(favFullName).first()).toBeVisible();
      await expect(page.getByText(plainFullName).first()).toBeVisible();

      // Flip "Show favorites": only the favorite remains.
      await page.getByLabel('Show favorites').click();
      await expect(page.getByText(favFullName).first()).toBeVisible();
      await expect(page.getByText(plainFullName).first()).not.toBeVisible();
    } finally {
      await deleteTestContact(request, favorite.ID);
      await deleteTestContact(request, plain.ID);
    }
  });

  test('shows favorited contacts in the dashboard favorites block', async ({ page, request }) => {
    const contact = await createTestContact(page.request, { lastname: 'FavDash' });
    try {
      const fav = await request.post(`${API_BASE_URL}/contacts/${contact.ID}/favorite`);
      expect(fav.ok()).toBeTruthy();

      await page.goto('/');
      await waitForLoading(page);

      // The Favorites block renders the favorited contact's name.
      await expect(page.getByText('Favorites').first()).toBeVisible();
      await expect(page.getByText(`${contact.firstname} FavDash`).first()).toBeVisible({
        timeout: 10000,
      });
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });
});
