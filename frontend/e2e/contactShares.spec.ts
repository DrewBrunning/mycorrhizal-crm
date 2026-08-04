import { test, expect, loginUser, LOGGED_OUT } from './fixtures';
import { createTestContact, deleteTestContact, searchContact, waitForLoading } from './fixtures';
import { API_BASE_URL } from './global-setup';

// A second, throwaway account used only to exercise the recipient side of a
// contact share (accept/decline/confirm). Mirrors isolation.spec.ts's own
// USER_B pattern -- registered idempotently, a 409 on re-run is fine.
const RECIPIENT = {
  username: 'e2e_share_recipient',
  email: 'e2e_share_recipient@example.com',
  password: 'ShareRecipientPass123!',
};

test.describe('Contact sharing', () => {
  test('share a contact; the recipient accepts it and it lands in their account with the shared field', async ({ page, browser }) => {
    await page.request.post(`${API_BASE_URL}/register`, { data: RECIPIENT }).catch(() => {});

    const contact = await createTestContact(page.request, {
      firstname: 'E2EShareAccept',
      lastname: `Test${Date.now()}`,
      email: 'e2e-share-accept@example.com',
    });
    const fullName = `${contact.firstname} ${contact.lastname}`;

    const recipientContext = await browser.newContext({ storageState: LOGGED_OUT });
    const recipientPage = await recipientContext.newPage();

    try {
      // Sender: open the contact, share it with the recipient.
      await page.goto(`/contacts/${contact.ID}`);
      await page.getByRole('button', { name: /^share contact$/i }).click();

      const shareDialog = page.getByRole('dialog');
      await expect(shareDialog).toBeVisible();
      await shareDialog.getByLabel(/recipient/i).click();
      await page.getByRole('option', { name: RECIPIENT.username, exact: true }).click();
      await shareDialog.getByRole('button', { name: /^share$/i }).click();
      await expect(shareDialog).toBeHidden();

      // Recipient: log in, see the incoming share, accept it. Scoped to
      // this share's own row -- the recipient account accumulates other
      // pending shares across repeated e2e runs, so an unscoped "Accept"
      // query would match more than one row.
      await loginUser(recipientPage, RECIPIENT);
      await recipientPage.goto('/shares');
      const shareRow = recipientPage.getByRole('listitem').filter({ hasText: fullName });
      await expect(shareRow).toBeVisible();
      await shareRow.getByRole('button', { name: /^accept$/i }).click();

      const reviewDialog = recipientPage.getByRole('dialog');
      await expect(reviewDialog).toBeVisible({ timeout: 10000 });
      // No existing match for this contact in a fresh account -- "add" is
      // the only/default action, and Confirm should be immediately usable.
      await expect(reviewDialog.getByRole('button', { name: /^confirm$/i })).toBeEnabled({ timeout: 10000 });
      await reviewDialog.getByRole('button', { name: /^confirm$/i }).click();
      await expect(reviewDialog).toBeHidden();
      await expect(shareRow.getByText('Accepted')).toBeVisible();

      // The recipient now has their own independent copy carrying the
      // shared field -- proven the same way contacts.spec.ts proves any
      // other contact's data landed: navigate and read it off the page.
      await recipientPage.goto('/contacts');
      await waitForLoading(recipientPage);
      await searchContact(recipientPage, fullName);
      await recipientPage.getByText(fullName).click();
      await expect(recipientPage.getByText('e2e-share-accept@example.com')).toBeVisible();

      // Clean up the recipient's independent copy too -- it has its own ID.
      await recipientPage.waitForURL(/\/contacts\/\d+/);
      const recipientContactID = recipientPage.url().match(/\/contacts\/(\d+)/)?.[1];
      if (recipientContactID) {
        await deleteTestContact(recipientPage.request, recipientContactID);
      }
    } finally {
      await recipientContext.close();
      await deleteTestContact(page.request, contact.ID);
    }
  });

  test('the recipient can decline a share; nothing is created and the sender keeps their copy', async ({ page, browser }) => {
    await page.request.post(`${API_BASE_URL}/register`, { data: RECIPIENT }).catch(() => {});

    const contact = await createTestContact(page.request, {
      firstname: 'E2EShareDecline',
      lastname: `Test${Date.now()}`,
    });
    const fullName = `${contact.firstname} ${contact.lastname}`;

    const recipientContext = await browser.newContext({ storageState: LOGGED_OUT });
    const recipientPage = await recipientContext.newPage();

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await page.getByRole('button', { name: /^share contact$/i }).click();
      const shareDialog = page.getByRole('dialog');
      await expect(shareDialog).toBeVisible();
      await shareDialog.getByLabel(/recipient/i).click();
      await page.getByRole('option', { name: RECIPIENT.username, exact: true }).click();
      await shareDialog.getByRole('button', { name: /^share$/i }).click();
      await expect(shareDialog).toBeHidden();

      await loginUser(recipientPage, RECIPIENT);
      await recipientPage.goto('/shares');
      const shareRow = recipientPage.getByRole('listitem').filter({ hasText: fullName });
      await expect(shareRow).toBeVisible();

      recipientPage.once('dialog', (d) => d.accept());
      await shareRow.getByRole('button', { name: /^decline$/i }).click();

      await expect(shareRow.getByText('Declined')).toBeVisible();

      // No duplicate contact should exist in the recipient's own account.
      await recipientPage.goto('/contacts');
      await waitForLoading(recipientPage);
      await searchContact(recipientPage, fullName);
      await expect(recipientPage.getByText(fullName)).not.toBeVisible();

      // The sender's original is untouched -- still reachable by ID.
      const senderResp = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      expect(senderResp.ok()).toBeTruthy();
    } finally {
      await recipientContext.close();
      await deleteTestContact(page.request, contact.ID);
    }
  });
});
