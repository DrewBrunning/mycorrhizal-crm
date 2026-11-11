import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import {
  acceptShareGrant,
  createShareGrant,
  declineShareGrant,
  deleteShareBlock,
  getShareGrantContent,
  getShareGrantNotifications,
  getShareJSContactPreview,
  leaveShareGrant,
  listIncomingShareGrants,
  listOutgoingShareGrants,
  listShareBlocks,
  markShareGrantNotificationsRead,
  reconfirmShareGrant,
  revokeShareGrant,
  updateShareGrant,
} from './shareGrants';

setupMswServer();

const owner = { id: 'g1', status: 'pending', sections: ['emails'], reconfirm_sections: [] };

describe('shareGrants api', () => {
  test('createShareGrant posts exactly the DTO fields and unwraps share_grant', async () => {
    const calls = mockApi('post', '/share-grants', { share_grant: owner });
    const out = await createShareGrant({
      to_user_id: 2,
      entity_type: 'contact',
      entity_id: 'uid-1',
      sections: ['emails'],
      expires_at: '2027-01-01T00:00:00Z',
    });
    expect(out.id).toBe('g1');
    expect(calls[0].body).toEqual({
      to_user_id: 2,
      entity_type: 'contact',
      entity_id: 'uid-1',
      sections: ['emails'],
      expires_at: '2027-01-01T00:00:00Z',
    });
  });

  test('createShareGrant sends the caller-supplied Idempotency-Key, and none otherwise', async () => {
    const calls = mockApi('post', '/share-grants', { share_grant: owner });
    await createShareGrant({ to_user_id: 2, entity_type: 'note', entity_id: 'n1' }, 'key-1');
    await createShareGrant({ to_user_id: 2, entity_type: 'note', entity_id: 'n1' });
    expect(calls[0].headers.get('Idempotency-Key')).toBe('key-1');
    expect(calls[1].headers.get('Idempotency-Key')).toBeNull();
  });

  test('createShareGrant surfaces a typed 409 for a duplicate live grant', async () => {
    mockApi('post', '/share-grants', errorEnvelope(409, 'CONFLICT', 'dup'));
    await expect(
      createShareGrant({ to_user_id: 2, entity_type: 'note', entity_id: 'n1' }),
    ).rejects.toMatchObject({ status: 409 });
  });

  test('list endpoints pass cursor/limit and return the page', async () => {
    const out = mockApi('get', '/share-grants/outgoing', {
      share_grants: [owner],
      next_cursor: 'c2',
      limit: 25,
    });
    const page = await listOutgoingShareGrants({ cursor: 'c1', limit: 25 });
    expect(page.share_grants).toHaveLength(1);
    expect(page.next_cursor).toBe('c2');
    expect(out[0].search.get('cursor')).toBe('c1');
    expect(out[0].search.get('limit')).toBe('25');

    mockApi('get', '/share-grants/incoming', { share_grants: [], next_cursor: '', limit: 25 });
    expect((await listIncomingShareGrants()).share_grants).toEqual([]);
  });

  test('a missing collection field is normalised to an empty list', async () => {
    mockApi('get', '/share-grants/incoming', {});
    expect((await listIncomingShareGrants()).share_grants).toEqual([]);
  });

  test('updateShareGrant sends PATCH with a quoted If-Match when a revision is known', async () => {
    const calls = mockApi('patch', '/share-grants/g1', { share_grant: owner });
    await updateShareGrant('g1', { sections: ['emails'], expires_at: null }, 4);
    expect(calls[0].headers.get('If-Match')).toBe('"4"');
    expect(calls[0].body).toEqual({ sections: ['emails'], expires_at: null });
  });

  test('updateShareGrant surfaces a 412 as a typed precondition failure', async () => {
    mockApi('patch', '/share-grants/g1', errorEnvelope(412, 'PRECONDITION_FAILED', 'stale'));
    await expect(updateShareGrant('g1', { expires_at: null }, 1)).rejects.toMatchObject({
      status: 412,
    });
  });

  test.each([
    ['accept', acceptShareGrant],
    ['leave', leaveShareGrant],
  ] as const)('%s posts to the action route and unwraps the view', async (action, fn) => {
    const calls = mockApi('post', `/share-grants/g1/${action}`, {
      share_grant: { id: 'g1', status: 'active' },
    });
    expect((await fn('g1')).id).toBe('g1');
    expect(calls[0].method).toBe('POST');
  });

  test.each([
    ['revoke', revokeShareGrant],
    ['reconfirm', reconfirmShareGrant],
  ] as const)('%s posts to the owner action route', async (action, fn) => {
    const calls = mockApi('post', `/share-grants/g1/${action}`, { share_grant: owner });
    expect((await fn('g1')).id).toBe('g1');
    expect(calls[0].method).toBe('POST');
  });

  test('declineShareGrant sends {block:true} only when asked to block', async () => {
    const calls = mockApi('post', '/share-grants/g1/decline', { share_grant: { id: 'g1' } });
    await declineShareGrant('g1');
    await declineShareGrant('g1', { block: true });
    expect(calls[0].body).toEqual({ block: false });
    expect(calls[1].body).toEqual({ block: true });
  });

  test('getShareGrantContent unwraps share_grant_content', async () => {
    mockApi('get', '/share-grants/g1/content', {
      share_grant_content: { grant_id: 'g1', entity_type: 'note', note: { content: 'hi' } },
    });
    expect((await getShareGrantContent('g1')).note?.content).toBe('hi');
  });

  test('block list and unblock', async () => {
    mockApi('get', '/share-grants/blocks', { blocks: [{ user_id: 3, username: 'eve' }] });
    expect(await listShareBlocks()).toEqual([{ user_id: 3, username: 'eve' }]);
    const calls = mockApi('delete', '/share-grants/blocks/3', new Response(null, { status: 204 }));
    await deleteShareBlock(3);
    expect(calls[0].method).toBe('DELETE');
  });

  test('notifications and mark-read', async () => {
    mockApi('get', '/share-grants/notifications', { pending_count: 2, notifications: [] });
    expect((await getShareGrantNotifications()).pending_count).toBe(2);
    const calls = mockApi('post', '/share-grants/notifications/read', { marked: 1 });
    await markShareGrantNotificationsRead();
    expect(calls[0].method).toBe('POST');
  });

  test('getShareJSContactPreview asks the export for exactly the chosen sections, never sensitive', async () => {
    const calls = mockApi('get', '/export/jscontact', [{ name: { full: 'Alice' } }]);
    const card = await getShareJSContactPreview('uid-1', ['emails', 'phones']);
    expect(card).toEqual({ name: { full: 'Alice' } });
    expect(calls[0].search.get('vcard_uid')).toBe('uid-1');
    expect(calls[0].search.get('sections')).toBe('emails,phones');
    expect(calls[0].search.get('include_sensitive')).toBeNull();
  });

  test('getShareJSContactPreview refuses an empty selection (the export treats none as ALL)', async () => {
    const calls = mockApi('get', '/export/jscontact', [{ name: { full: 'Alice' } }]);
    await expect(getShareJSContactPreview('uid-1', [])).rejects.toThrow();
    expect(calls).toHaveLength(0);
  });
});
