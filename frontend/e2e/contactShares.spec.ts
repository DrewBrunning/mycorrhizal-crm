import { test, expect, loginUser, LOGGED_OUT } from './fixtures';
import { createTestContact, deleteTestContact, searchContact, waitForLoading, makeThrowawayUser, deleteThrowawayUser } from './fixtures';
import { API_BASE_URL } from './global-setup';

// Each test below registers its own uniquely-suffixed throwaway recipient
// account, rather than sharing one fixed username across both tests: this
// file has no serial mode, so under `fullyParallel: true` both tests can run
// at once, and a fixed shared recipient account deleted by one test's
// cleanup while the other is still mid-login/accept would be exactly the
// kind of cross-test race this whole review pass exists to close. A
// per-test account also means it can be hard-deleted in that test's own
// `finally` instead of accumulating forever (mirrors isolation.spec.ts).

test.describe('Contact sharing', () => {
  test('share a contact; the recipient accepts it and it lands in their account with the shared field', async ({ page, browser }) => {
    const recipient = makeThrowawayUser('share_recipient');
    const registered = await page.request.post(`${API_BASE_URL}/register`, { data: recipient });
    expect(registered.ok(), `recipient registration should succeed: ${await registered.text()}`).toBeTruthy();

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
      await page.getByRole('option', { name: recipient.username, exact: true }).click();
      await shareDialog.getByRole('button', { name: /^share$/i }).click();
      await expect(shareDialog).toBeHidden();

      // Recipient: log in, see the incoming share, accept it. The account is
      // fresh (created above just for this test), but the row is still
      // scoped to this share by name defensively.
      await loginUser(recipientPage, recipient);
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
      await deleteThrowawayUser(page.request, recipient.username);
    }
  });

  test('the recipient can decline a share; nothing is created and the sender keeps their copy', async ({ page, browser }) => {
    const recipient = makeThrowawayUser('share_recipient');
    const registered = await page.request.post(`${API_BASE_URL}/register`, { data: recipient });
    expect(registered.ok(), `recipient registration should succeed: ${await registered.text()}`).toBeTruthy();

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
      await page.getByRole('option', { name: recipient.username, exact: true }).click();
      await shareDialog.getByRole('button', { name: /^share$/i }).click();
      await expect(shareDialog).toBeHidden();

      await loginUser(recipientPage, recipient);
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
      await deleteThrowawayUser(page.request, recipient.username);
    }
  });
});
