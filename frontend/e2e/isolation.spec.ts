import { test, expect } from './fixtures';
import { createTestContact, deleteTestContact } from './fixtures';
import { request } from '@playwright/test';
import { API_BASE_URL } from './global-setup';

// A second account used only to prove data isolation (a 409 on re-run is fine).
const USER_B = {
  username: 'e2e_isolation_userb',
  email: 'e2e_isolation_userb@example.com',
  password: 'IsolationPass123!',
};

// Two more throwaway accounts for the contact-share isolation test below --
// distinct from USER_B so the two tests' registrations can't interfere.
const SHARE_RECIPIENT = {
  username: 'e2e_isolation_share_recipient',
  email: 'e2e_isolation_share_recipient@example.com',
  password: 'ShareRecipientPass123!',
};
const SHARE_THIRD_PARTY = {
  username: 'e2e_isolation_share_thirdparty',
  email: 'e2e_isolation_share_thirdparty@example.com',
  password: 'ShareThirdPartyPass123!',
};

test.describe('Multi-user isolation', () => {
  test('a user cannot see another user\'s contacts', async ({ page }) => {
    // Sanity: the seeded user (userA, via the shared storageState) can see Alice.
    const ownView = await page.request.get(
      `${API_BASE_URL}/contacts?search=${encodeURIComponent('Alice Johnson')}&limit=10`
    );
    expect(ownView.ok()).toBeTruthy();
    const own = await ownView.json();
    expect(
      (own.contacts || []).some((c: any) => c.firstname === 'Alice' && c.lastname === 'Johnson')
    ).toBeTruthy();

    // userB gets a clean API context with no shared cookies.
    const ctx = await request.newContext();
    try {
      await ctx.post(`${API_BASE_URL}/register`, { data: USER_B }).catch(() => {});

      const login = await ctx.post(`${API_BASE_URL}/login`, {
        data: { identifier: USER_B.username, password: USER_B.password },
      });
      expect(login.ok(), 'userB login should succeed').toBeTruthy();

      // userB must not see any of userA's seeded contacts.
      const search = await ctx.get(
        `${API_BASE_URL}/contacts?search=${encodeURIComponent('Alice Johnson')}&limit=10`
      );
      expect(search.ok()).toBeTruthy();
      const result = await search.json();
      expect(
        (result.contacts || []).some((c: any) => c.firstname === 'Alice' && c.lastname === 'Johnson')
      ).toBeFalsy();
    } finally {
      await ctx.dispose();
    }
  });

  test('a third user cannot see a contact share between two other users', async ({ page }) => {
    const recipientCtx = await request.newContext();
    const thirdCtx = await request.newContext();
    let contact: Awaited<ReturnType<typeof createTestContact>> | undefined;

    try {
      await recipientCtx.post(`${API_BASE_URL}/register`, { data: SHARE_RECIPIENT }).catch(() => {});
      const recipientLogin = await recipientCtx.post(`${API_BASE_URL}/login`, {
        data: { identifier: SHARE_RECIPIENT.username, password: SHARE_RECIPIENT.password },
      });
      expect(recipientLogin.ok(), 'recipient login should succeed').toBeTruthy();

      await thirdCtx.post(`${API_BASE_URL}/register`, { data: SHARE_THIRD_PARTY }).catch(() => {});
      const thirdLogin = await thirdCtx.post(`${API_BASE_URL}/login`, {
        data: { identifier: SHARE_THIRD_PARTY.username, password: SHARE_THIRD_PARTY.password },
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
        (u: any) => u.username === SHARE_RECIPIENT.username
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
    }
  });
});
