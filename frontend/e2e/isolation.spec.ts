import { type Page, request } from '@playwright/test';
import {
  createTestContact,
  deleteTestContact,
  deleteThrowawayUser,
  expect,
  makeThrowawayUser,
  test,
} from './fixtures';
import { API_BASE_URL } from './global-setup';
import { deleteAccountAsAdmin, provisionWorkerUser, workerUserCredentials } from './workerUser';

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
      await deleteThrowawayUser(userB.username);
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
      await deleteThrowawayUser(shareRecipient.username);
      await deleteThrowawayUser(shareThirdParty.username);
    }
  });

  // Issue #1480: the harness's own isolation. Every Playwright worker
  // authenticates as a distinct account, and a test mutating or creating data
  // as one must be invisible to every other -- if this ever failed, the
  // per-worker-user model (the replacement for the old settings lock) would
  // be silently sharing state again.
  test("per-worker e2e users never see each other's contacts or settings", async ({
    page,
    browser,
    workerUser,
  }, testInfo) => {
    // Provisioning the sibling account (register + seed + UI login) is itself
    // several seconds of work, well beyond what a typical API-only test spends.
    test.setTimeout(90_000);

    // Not the shared admin, and named for this worker.
    expect(workerUser.username).not.toBe('testuser');
    expect(workerUser.username).toBe(workerUserCredentials(testInfo.workerIndex).username);

    // A second worker-style account, provisioned exactly as a sibling worker's
    // would be (distinct index), stands in for "another worker's user".
    const sibling = await provisionWorkerUser(
      browser,
      10_000 + testInfo.workerIndex,
      testInfo.outputPath('sibling-auth'),
    );
    const siblingCtx = await request.newContext({
      baseURL: 'http://localhost:7300',
      storageState: sibling.storageStatePath,
    });
    const marker = `E2EFixtureIso${Date.now()}`;
    let mine: Awaited<ReturnType<typeof createTestContact>> | undefined;
    let theirs: Awaited<ReturnType<typeof createTestContact>> | undefined;
    const count = async (api: typeof page.request, q: string) => {
      const r = await api.get(`${API_BASE_URL}/contacts?search=${encodeURIComponent(q)}&limit=50`);
      expect(r.ok()).toBeTruthy();
      return ((await r.json()).contacts || []).length;
    };
    try {
      mine = await createTestContact(page.request, { firstname: marker, lastname: 'Mine' });
      theirs = await createTestContact(siblingCtx, { firstname: marker, lastname: 'Theirs' });

      // Each sees exactly its own marker contact -- never the other's.
      expect(await count(page.request, marker)).toBe(1);
      expect(await count(siblingCtx, marker)).toBe(1);
      expect(await count(page.request, 'Theirs')).toBe(0);
      expect(await count(siblingCtx, 'Mine')).toBe(0);

      // Both are seeded with the same sample dataset, independently.
      expect(await count(page.request, 'Alice')).toBeGreaterThan(0);
      expect(await count(siblingCtx, 'Alice')).toBeGreaterThan(0);

      // A settings write by one never reaches the other (the old shared-user
      // corruption this harness exists to prevent).
      const patch = await siblingCtx.patch(`${API_BASE_URL}/users/enabled-contact-fields`, {
        data: { fields: ['socialProfiles'] },
      });
      expect(patch.ok(), `enabled-contact-fields PATCH: ${patch.status()}`).toBeTruthy();
      const theirFields = (
        await (await siblingCtx.get(`${API_BASE_URL}/users/enabled-contact-fields`)).json()
      ).enabled_contact_fields;
      const myFields = (
        await (await page.request.get(`${API_BASE_URL}/users/enabled-contact-fields`)).json()
      ).enabled_contact_fields;
      expect(theirFields).toEqual(['socialProfiles']);
      expect(myFields).not.toEqual(['socialProfiles']);
    } finally {
      if (mine) await deleteTestContact(page.request, mine.ID);
      if (theirs) await deleteTestContact(siblingCtx, theirs.ID);
      await siblingCtx.dispose();
      await deleteAccountAsAdmin(sibling.username);
    }
  });
});

// Issue #1480: the shared `testuser` is the read-only-by-contract user, and the
// fixture that enforces it is itself under test.
test.describe('Shared read-only user guard', () => {
  test.use({ sharedUser: true });

  const patchLanguage = (page: Page) =>
    page.evaluate(async () => {
      try {
        const res = await fetch('/api/v1/users/language', {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ language: 'en' }),
        });
        return res.status;
      } catch {
        return 'blocked';
      }
    });

  test('a browser write to an account setting never reaches the server and fails the test', async ({
    page,
  }) => {
    // test.fail() inverts the verdict: this passes only if the guard in
    // fixtures.ts' `context` override records the write and fails the test.
    test.fail();
    await page.goto('/');
    expect(await patchLanguage(page)).toBe('blocked');
  });

  test('reads of account settings are unaffected', async ({ page }) => {
    await page.goto('/');
    const status = await page.evaluate(
      async () => (await fetch('/api/v1/users/enabled-contact-fields')).status,
    );
    expect(status).toBe(200);
  });
});
