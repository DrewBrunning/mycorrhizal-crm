import { request } from '@playwright/test';
import {
  createTestContact,
  deleteTestContact,
  deleteThrowawayUser,
  expect,
  makeThrowawayUser,
  test,
} from './fixtures';
import { API_BASE_URL } from './global-setup';

test.describe('Multi-user isolation', () => {
  test("a user cannot see another user's contacts", async ({ page }) => {
    // Sanity: the seeded user (userA, via the shared storageState) can see Alice.
    const ownView = await page.request.get(
      `${API_BASE_URL}/contacts?search=${encodeURIComponent('Alice Johnson')}&limit=10`,
    );
    expect(ownView.ok()).toBeTruthy();
    const own = await ownView.json();
    expect(
      (own.contacts || []).some((c: any) => c.firstname === 'Alice' && c.lastname === 'Johnson'),
    ).toBeTruthy();

    // A throwaway account used only to prove data isolation, uniquely
    // suffixed (rather than a fixed username) so it never collides with
    // another run and can be hard-deleted below instead of accumulating.
    const userB = makeThrowawayUser('isolation_userb');
    // userB gets a clean API context with no shared cookies.
    const ctx = await request.newContext();
    try {
      const registered = await ctx.post(`${API_BASE_URL}/register`, { data: userB });
      expect(
        registered.ok(),
        `userB registration should succeed: ${await registered.text()}`,
      ).toBeTruthy();

      const login = await ctx.post(`${API_BASE_URL}/login`, {
        data: { identifier: userB.username, password: userB.password },
      });
      expect(login.ok(), 'userB login should succeed').toBeTruthy();

      // userB must not see any of userA's seeded contacts.
      const search = await ctx.get(
        `${API_BASE_URL}/contacts?search=${encodeURIComponent('Alice Johnson')}&limit=10`,
      );
      expect(search.ok()).toBeTruthy();
      const result = await search.json();
      expect(
        (result.contacts || []).some(
          (c: any) => c.firstname === 'Alice' && c.lastname === 'Johnson',
        ),
      ).toBeFalsy();
    } finally {
      await ctx.dispose();
      // page.request is authenticated as the shared (auto-admin) TEST_USER.
      await deleteThrowawayUser(page.request, userB.username);
    }
  });

  test('a third user cannot see a contact share between two other users', async ({ page }) => {
    // Distinct throwaway accounts, uniquely suffixed so they never collide
    // with another run's or the other test's own accounts here.
    const shareRecipient = makeThrowawayUser('isolation_share_recipient');
    const shareThirdParty = makeThrowawayUser('isolation_share_thirdparty');
    const recipientCtx = await request.newContext();
    const thirdCtx = await request.newContext();
    let contact: Awaited<ReturnType<typeof createTestContact>> | undefined;

    try {
      const recipientRegistered = await recipientCtx.post(`${API_BASE_URL}/register`, {
        data: shareRecipient,
      });
      expect(
        recipientRegistered.ok(),
        `recipient registration should succeed: ${await recipientRegistered.text()}`,
      ).toBeTruthy();
      const recipientLogin = await recipientCtx.post(`${API_BASE_URL}/login`, {
        data: { identifier: shareRecipient.username, password: shareRecipient.password },
      });
      expect(recipientLogin.ok(), 'recipient login should succeed').toBeTruthy();

      const thirdRegistered = await thirdCtx.post(`${API_BASE_URL}/register`, {
        data: shareThirdParty,
      });
      expect(
        thirdRegistered.ok(),
        `third party registration should succeed: ${await thirdRegistered.text()}`,
      ).toBeTruthy();
      const thirdLogin = await thirdCtx.post(`${API_BASE_URL}/login`, {
        data: { identifier: shareThirdParty.username, password: shareThirdParty.password },
      });
      expect(thirdLogin.ok(), 'third party login should succeed').toBeTruthy();

      // Sender (the shared storageState user) creates a contact and shares
      // it with the recipient.
      contact = await createTestContact(page.request, {
        firstname: 'E2EIsolationShare',
        lastname: `Test${Date.now()}`,
      });

      const directory = await page.request.get(`${API_BASE_URL}/users/directory`);
      expect(directory.ok()).toBeTruthy();
      const directoryBody = await directory.json();
      const recipientEntry = (directoryBody.users || []).find(
        (u: any) => u.username === shareRecipient.username,
      );
      expect(recipientEntry, 'recipient should be discoverable via /users/directory').toBeTruthy();

      const shareResp = await page.request.post(`${API_BASE_URL}/contact-shares`, {
        data: { to_user_id: recipientEntry.id, vcard_uid: contact.uid, sections: ['emails'] },
      });
      expect(shareResp.ok(), `share creation failed: ${await shareResp.text()}`).toBeTruthy();

      const isShared = (shares: any[]) =>
        (shares || []).some((s: any) => s.contact_display_name?.includes('E2EIsolationShare'));

      // The recipient sees it incoming.
      const incoming = await recipientCtx.get(`${API_BASE_URL}/contact-shares/incoming`);
      expect(incoming.ok()).toBeTruthy();
      expect(isShared((await incoming.json()).contact_shares)).toBeTruthy();

      // The uninvolved third party sees neither side.
      const thirdIncoming = await thirdCtx.get(`${API_BASE_URL}/contact-shares/incoming`);
      expect(isShared((await thirdIncoming.json()).contact_shares)).toBeFalsy();

      const thirdOutgoing = await thirdCtx.get(`${API_BASE_URL}/contact-shares/outgoing`);
      expect(isShared((await thirdOutgoing.json()).contact_shares)).toBeFalsy();
    } finally {
      await recipientCtx.dispose();
      await thirdCtx.dispose();
      if (contact) await deleteTestContact(page.request, contact.ID);
      await deleteThrowawayUser(page.request, shareRecipient.username);
      await deleteThrowawayUser(page.request, shareThirdParty.username);
    }
  });
});
