import type { Page } from '@playwright/test';
import {
  createTestContact,
  deleteTestContact,
  deleteThrowawayUser,
  expect,
  LOGGED_OUT,
  loginUser,
  makeThrowawayUser,
  searchContact,
  test,
  waitForLoading,
} from './fixtures';
import { expectFocusNotLost } from './focusOrder';
import { API_BASE_URL } from './global-setup';

/** The id of a registered user as the worker's own directory lookup sees it. */
async function directoryUserID(page: Page, username: string): Promise<number> {
  const directory = await page.request.get(`${API_BASE_URL}/users/directory`);
  expect(directory.ok(), `directory lookup failed: ${await directory.text()}`).toBeTruthy();
  const users = (await directory.json()).users as { id: number; username: string }[];
  const found = users.find((u) => u.username === username);
  expect(found, `recipient ${username} must appear in the share directory`).toBeTruthy();
  return found?.id ?? 0;
}

/** Registers a throwaway recipient so the worker can address a standing-share grant to it. */
async function registerShareRecipient(page: Page): Promise<ReturnType<typeof makeThrowawayUser>> {
  const recipient = makeThrowawayUser('standing_share_recipient');
  const registered = await page.request.post(`${API_BASE_URL}/register`, { data: recipient });
  expect(
    registered.ok(),
    `recipient registration should succeed: ${await registered.text()}`,
  ).toBeTruthy();
  return recipient;
}

// Each test below registers its own uniquely-suffixed throwaway recipient
// account, rather than sharing one fixed username across both tests: this
// file has no serial mode, so under `fullyParallel: true` both tests can run
// at once, and a fixed shared recipient account deleted by one test's
// cleanup while the other is still mid-login/accept would be exactly the
// kind of cross-test race this whole review pass exists to close. A
// per-test account also means it can be hard-deleted in that test's own
// `finally` instead of accumulating forever (mirrors isolation.spec.ts).

test.describe('Contact sharing', () => {
  test('share a contact; the recipient accepts it and it lands in their account with the shared field', async ({
    page,
    browser,
  }) => {
    const recipient = makeThrowawayUser('share_recipient');
    const registered = await page.request.post(`${API_BASE_URL}/register`, { data: recipient });
    expect(
      registered.ok(),
      `recipient registration should succeed: ${await registered.text()}`,
    ).toBeTruthy();

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
      await expect(reviewDialog.getByRole('button', { name: /^confirm$/i })).toBeEnabled({
        timeout: 10000,
      });
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
      await deleteThrowawayUser(recipient.username);
    }
  });

  test('the recipient can decline a share; nothing is created and the sender keeps their copy', async ({
    page,
    browser,
  }) => {
    const recipient = makeThrowawayUser('share_recipient');
    const registered = await page.request.post(`${API_BASE_URL}/register`, { data: recipient });
    expect(
      registered.ok(),
      `recipient registration should succeed: ${await registered.text()}`,
    ).toBeTruthy();

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
      // The empty-state message itself quotes the query ("No results for
      // "<fullName>"."), so a bare getByText(fullName) matches the message and
      // can never be "not visible" once the search settles (and passes
      // vacuously before it does). Wait for the settled empty state, then
      // assert no contact card carries the exact name.
      await expect(recipientPage.getByText(/^No results for "/)).toBeVisible();
      await expect(recipientPage.getByText(fullName, { exact: true })).toHaveCount(0);

      // The sender's original is untouched -- still reachable by ID.
      const senderResp = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      expect(senderResp.ok()).toBeTruthy();
    } finally {
      await recipientContext.close();
      await deleteTestContact(page.request, contact.ID);
      await deleteThrowawayUser(recipient.username);
    }
  });

  test('removing an ended share keeps focus on the page', async ({ page }) => {
    const recipient = await registerShareRecipient(page);
    const contact = await createTestContact(page.request, {
      firstname: 'E2EShareRemove',
      lastname: `Test${Date.now()}`,
    });
    const fullName = `${contact.firstname} ${contact.lastname}`;

    try {
      const toUserID = await directoryUserID(page, recipient.username);
      const created = await page.request.post(`${API_BASE_URL}/share-grants`, {
        data: {
          to_user_id: toUserID,
          entity_type: 'contact',
          entity_id: contact.uid,
          sections: ['emails'],
        },
      });
      expect(created.ok(), `grant create failed: ${await created.text()}`).toBeTruthy();
      const grant = (await created.json()).share_grant as { id: string };
      const revoked = await page.request.post(`${API_BASE_URL}/share-grants/${grant.id}/revoke`);
      expect(revoked.ok(), `grant revoke failed: ${await revoked.text()}`).toBeTruthy();

      await page.goto('/shared-by-me');
      await expect(page.getByRole('heading', { level: 1, name: 'Shared by me' })).toBeVisible();
      await page.getByRole('button', { name: /^Ended shares/ }).click();

      const row = page.getByRole('listitem').filter({ hasText: fullName });
      await expect(row).toBeVisible();
      await row.getByRole('button', { name: /^Remove share of/ }).click();

      const confirm = page.getByRole('dialog');
      await expect(confirm.getByRole('heading', { name: 'Remove this share?' })).toBeVisible();
      await confirm.getByRole('button', { name: 'Remove', exact: true }).click();
      await expect(confirm).toBeHidden();
      await expect(row).toHaveCount(0);

      // #2920 item 7: the removed row held focus; it must not fall to <body>.
      await expectFocusNotLost(page, 'share Remove');
    } finally {
      await deleteTestContact(page.request, contact.ID);
      await deleteThrowawayUser(recipient.username);
    }
  });

  // #2920 follow-up: Revoke moves its row out of the live list; the Revoke
  // button that held focus unmounts, and focus used to fall to <body>.
  test('revoking a live share keeps focus on the page', async ({ page }) => {
    const recipient = await registerShareRecipient(page);
    const contact = await createTestContact(page.request, {
      firstname: 'E2EShareRevoke',
      lastname: `Test${Date.now()}`,
    });
    const fullName = `${contact.firstname} ${contact.lastname}`;

    try {
      const toUserID = await directoryUserID(page, recipient.username);
      const created = await page.request.post(`${API_BASE_URL}/share-grants`, {
        data: {
          to_user_id: toUserID,
          entity_type: 'contact',
          entity_id: contact.uid,
          sections: ['emails'],
        },
      });
      expect(created.ok(), `grant create failed: ${await created.text()}`).toBeTruthy();

      await page.goto('/shared-by-me');
      await expect(page.getByRole('heading', { level: 1, name: 'Shared by me' })).toBeVisible();
      const row = page.getByRole('listitem').filter({ hasText: fullName });
      await row.getByRole('button', { name: /^Revoke / }).click();

      const confirm = page.getByRole('dialog');
      await expect(confirm.getByRole('heading', { name: 'Revoke this share?' })).toBeVisible();
      await confirm.getByRole('button', { name: 'Revoke', exact: true }).click();
      await expect(confirm).toBeHidden();
      await expect(row.getByRole('button', { name: /^Revoke / })).toHaveCount(0);

      await expectFocusNotLost(page, 'share Revoke');
    } finally {
      await deleteTestContact(page.request, contact.ID);
      await deleteThrowawayUser(recipient.username);
    }
  });

  test('re-confirming a share keeps focus on the page', async ({ page }) => {
    const recipient = await registerShareRecipient(page);
    const contact = await createTestContact(page.request, {
      firstname: 'E2EShareReconfirm',
      lastname: `Test${Date.now()}`,
      addresses: [
        {
          type: 'home',
          street: '1 Reconfirm Street',
          city: 'Testville',
          region: '',
          postal: '',
          country: '',
          sensitivity: 'normal',
        },
      ],
    });
    const fullName = `${contact.firstname} ${contact.lastname}`;
    let grantID = '';

    try {
      const toUserID = await directoryUserID(page, recipient.username);
      const created = await page.request.post(`${API_BASE_URL}/share-grants`, {
        data: {
          to_user_id: toUserID,
          entity_type: 'contact',
          entity_id: contact.uid,
          sections: ['addresses'],
        },
      });
      expect(created.ok(), `grant create failed: ${await created.text()}`).toBeTruthy();
      const grant = (await created.json()).share_grant as { id: string };
      grantID = grant.id;

      // Reclassify the shared address to private: the grant must withhold the
      // section until the owner re-confirms (ADR 0042 decision 5).
      const read = await page.request.get(`${API_BASE_URL}/contacts/${contact.ID}`);
      const body = await read.json();
      const record = body.contact ?? body;
      record.card.addresses[0].sensitivity = 'private';
      const save = await page.request.put(`${API_BASE_URL}/contacts/${contact.ID}`, {
        data: record,
      });
      expect(save.ok(), `contact update failed: ${await save.text()}`).toBeTruthy();

      await expect
        .poll(
          async () => {
            const response = await page.request.get(`${API_BASE_URL}/share-grants/outgoing`);
            const grants = (await response.json()).share_grants as {
              id: string;
              reconfirm_sections: string[];
            }[];
            return grants.find((g) => g.id === grant.id)?.reconfirm_sections ?? [];
          },
          { message: 'the reclassified address must withhold the grant section', timeout: 10000 },
        )
        .toContain('addresses');

      await page.goto('/shared-by-me');
      const row = page.getByRole('listitem').filter({ hasText: fullName });
      await expect(row).toBeVisible();
      const reconfirm = row.getByRole('button', { name: /^Review and re-confirm/ });
      await reconfirm.click();

      const confirm = page.getByRole('dialog');
      await expect(
        confirm.getByRole('heading', { name: 'Share these sections again?' }),
      ).toBeVisible();
      await confirm.getByRole('button', { name: 'Confirm', exact: true }).click();
      await expect(confirm).toBeHidden();
      await expect(reconfirm).toHaveCount(0);

      // #2920 item 7: confirming replaced the row and removed the button that
      // had focus; it must not fall to <body>.
      await expectFocusNotLost(page, 'share re-confirm');
    } finally {
      // A live grant refuses DELETE; revoke it first, then dismiss the ended row.
      if (grantID) {
        await page.request.post(`${API_BASE_URL}/share-grants/${grantID}/revoke`).catch(() => {});
        await page.request.delete(`${API_BASE_URL}/share-grants/${grantID}`).catch(() => {});
      }
      await deleteTestContact(page.request, contact.ID);
      await deleteThrowawayUser(recipient.username);
    }
  });
});
