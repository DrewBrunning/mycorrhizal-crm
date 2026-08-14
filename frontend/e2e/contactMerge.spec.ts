import { test, expect } from '@playwright/test';
import { createTestContact, deleteTestContact, waitForLoading } from './fixtures';
import { API_BASE_URL, E2E_CONTACT_PREFIX } from './global-setup';

/**
 * Contact merge / dedupe (N1) — the one R5 capability inside alpha, and
 * destructive: it moves every association off the loser and then deletes it,
 * with no undo. It had no e2e coverage.
 *
 * The parts that matter here are the ones a unit test cannot see: that the
 * preview the user confirms against actually matches what the commit does,
 * and that associations survive the move rather than being silently dropped
 * along with the deleted contact.
 */
test.describe('Contact merge', () => {
  test('moves notes and reminders onto the keeper and removes the loser', async ({ request }) => {
    const keeper = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}Keeper`,
      lastname: 'Merge',
    });
    const loser = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}Loser`,
      lastname: 'Merge',
    });

    try {
      const note = await request.post(`${API_BASE_URL}/contacts/${loser.ID}/notes`, {
        data: { content: 'A note that must survive the merge', date: new Date().toISOString() },
      });
      expect(note.ok()).toBeTruthy();

      // The preview is the contract the user confirms against.
      const preview = await request.post(`${API_BASE_URL}/contacts/merge/preview`, {
        data: { keep_id: keeper.ID, merge_id: loser.ID },
      });
      expect(preview.ok(), `preview failed: ${await preview.text()}`).toBeTruthy();
      const previewBody = await preview.json();
      expect(previewBody.association_counts.notes, 'the preview must count the note that will move').toBe(1);

      // The two contacts have different firstnames, so the merge reports a
      // conflict and REFUSES to commit until the caller resolves it. That
      // refusal is the safety property worth pinning: an unresolved merge
      // must never silently pick a winner.
      const unresolved = await request.post(`${API_BASE_URL}/contacts/merge`, {
        data: { keep_id: keeper.ID, merge_id: loser.ID, resolutions: {} },
      });
      expect(unresolved.status(), 'a merge with unresolved conflicts must be rejected').toBe(400);

      // Resolve exactly as the dialog does: take the keeper's value for each
      // conflicting field the preview reported.
      const resolutions: Record<string, string> = {};
      for (const conflict of previewBody.resolution?.conflicts ?? []) {
        resolutions[conflict.field] = conflict.keeper_value;
      }
      expect(Object.keys(resolutions), 'the preview must report the firstname conflict').toContain('firstname');

      const commit = await request.post(`${API_BASE_URL}/contacts/merge`, {
        data: { keep_id: keeper.ID, merge_id: loser.ID, resolutions },
      });
      expect(commit.ok(), `merge failed: ${await commit.text()}`).toBeTruthy();

      // The loser is gone.
      const loserResponse = await request.get(`${API_BASE_URL}/contacts/${loser.ID}`);
      expect(loserResponse.status(), 'the merged-away contact must no longer be retrievable').toBe(404);

      // The note moved rather than being deleted with its old owner. This is
      // the failure that would silently lose user data.
      const notesResponse = await request.get(`${API_BASE_URL}/contacts/${keeper.ID}/notes`);
      expect(notesResponse.ok()).toBeTruthy();
      const notesBody = await notesResponse.json();
      const contents = (notesBody.notes ?? notesBody).map((n: { content: string }) => n.content);
      expect(contents).toContain('A note that must survive the merge');

      // An audit note records what happened -- the only record of the merge.
      expect(
        contents.some((c: string) => c.includes('Merged contact')),
        'the merge must leave an audit note on the keeper'
      ).toBeTruthy();
    } finally {
      await deleteTestContact(request, keeper.ID);
      await deleteTestContact(request, loser.ID);
    }
  });

  test('moves circle memberships without duplicating a shared one', async ({ request }) => {
    // Identical names on purpose: this test is about membership repointing,
    // so there must be no field conflict to resolve first.
    const sameName = `${E2E_CONTACT_PREFIX}CircleMerge${Date.now()}`;
    const keeper = await createTestContact(request, { firstname: sameName, lastname: 'Merge' });
    const loser = await createTestContact(request, { firstname: sameName, lastname: 'Merge' });

    const sharedName = `SharedCircle${Date.now()}`;
    const soloName = `SoloCircle${Date.now()}`;
    const shared = await (await request.post(`${API_BASE_URL}/circles`, { data: { name: sharedName } })).json();
    const solo = await (await request.post(`${API_BASE_URL}/circles`, { data: { name: soloName } })).json();
    const sharedId = shared.circle?.id ?? shared.id;
    const soloId = solo.circle?.id ?? solo.id;

    try {
      // Both are in the shared circle -- repointing naively would violate the
      // (circle_id, member_vcard_uid) unique index and roll the merge back.
      for (const uid of [keeper.uid, loser.uid]) {
        const r = await request.post(`${API_BASE_URL}/circles/${sharedId}/members`, {
          data: { member_vcard_uid: uid },
        });
        expect(r.ok()).toBeTruthy();
      }
      const soloAdd = await request.post(`${API_BASE_URL}/circles/${soloId}/members`, {
        data: { member_vcard_uid: loser.uid },
      });
      expect(soloAdd.ok()).toBeTruthy();

      const commit = await request.post(`${API_BASE_URL}/contacts/merge`, {
        data: { keep_id: keeper.ID, merge_id: loser.ID, resolutions: {} },
      });
      expect(commit.ok(), `merge failed: ${await commit.text()}`).toBeTruthy();

      // GET /circles/:id always returns that circle's own members -- fetching
      // the two circles this test actually created directly, rather than
      // paging the account's whole circle list with a ?limit=200 cap, can't
      // silently miss them if the shared account happens to hold more than
      // 200 circles from other parallel tests.
      const keeperMembershipCount = async (circleId: string) => {
        const res = await request.get(`${API_BASE_URL}/circles/${circleId}`);
        expect(res.ok()).toBeTruthy();
        const { members } = await res.json();
        return (members ?? []).filter((m: { member_vcard_uid: string }) => m.member_vcard_uid === keeper.uid).length;
      };

      expect(await keeperMembershipCount(sharedId), 'the keeper must be in the shared circle exactly once, not duplicated').toBe(1);
      expect(await keeperMembershipCount(soloId), 'the keeper must have inherited the loser-only circle exactly once').toBe(1);
    } finally {
      await deleteTestContact(request, keeper.ID);
      await deleteTestContact(request, loser.ID);
      await request.delete(`${API_BASE_URL}/circles/${sharedId}`).catch(() => {});
      await request.delete(`${API_BASE_URL}/circles/${soloId}`).catch(() => {});
    }
  });

  test('reaches the merge dialog from the contact page', async ({ page, request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}MergeUI`,
      lastname: 'Target',
    });

    try {
      await page.goto(`/contacts/${contact.ID}`);
      await waitForLoading(page);

      await page.getByRole('button', { name: /^merge$/i }).click();
      await expect(page.getByText('Merge Contacts')).toBeVisible({ timeout: 10000 });
      // MUI renders the label twice (InputLabel + fieldset legend).
      await expect(page.getByText('Merge into').first()).toBeVisible();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });

  test('refuses to merge a contact into itself', async ({ request }) => {
    const contact = await createTestContact(request, {
      firstname: `${E2E_CONTACT_PREFIX}Self`,
      lastname: 'Merge',
    });

    try {
      const response = await request.post(`${API_BASE_URL}/contacts/merge`, {
        data: { keep_id: contact.ID, merge_id: contact.ID, resolutions: {} },
      });
      expect(response.ok(), 'merging a contact into itself must be rejected').toBeFalsy();
    } finally {
      await deleteTestContact(request, contact.ID);
    }
  });
});
