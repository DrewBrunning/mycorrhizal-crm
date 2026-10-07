import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { AccountDeletionRequiresPromotionError, deleteOwnAccount } from './auth';
import { formatSuggestionAddress } from './dataSuggestions';

setupMswServer();

const url = '/account';

describe('deleteOwnAccount error mapping', () => {
  test('success without a message body falls back to the default confirmation', async () => {
    mockApi('delete', url, new Response(null, { status: 204 }));
    expect(await deleteOwnAccount('pw')).toBe('Your account and all its data have been deleted.');
  });

  test('a 409 with candidates throws AccountDeletionRequiresPromotionError carrying them', async () => {
    const res = new Response(
      JSON.stringify({
        error: {
          code: 'CONFLICT',
          message: 'Pick an admin',
          details: {
            candidates: [{ id: 2, username: 'bob' }, { id: 'x', username: 'bad' }, null, { id: 3 }],
          },
        },
      }),
      { status: 409 },
    );
    mockApi('delete', url, res);
    const err = await deleteOwnAccount('pw').catch((e) => e);
    expect(err).toBeInstanceOf(AccountDeletionRequiresPromotionError);
    expect(err.message).toBe('Pick an admin');
    expect(err.candidates).toEqual([{ id: 2, username: 'bob' }]);
  });

  test('a 409 with candidates and no message uses the default prompt', async () => {
    mockApi(
      'delete',
      url,
      new Response(
        JSON.stringify({ error: { details: { candidates: [{ id: 1, username: 'a' }] } } }),
        {
          status: 409,
        },
      ),
    );
    const err = await deleteOwnAccount('pw').catch((e) => e);
    expect(err).toBeInstanceOf(AccountDeletionRequiresPromotionError);
    expect(err.message).toBe(
      'Choose another user to promote to admin before deleting your account.',
    );
  });

  test('a 409 without a candidates array is an ordinary error', async () => {
    mockApi('delete', url, errorEnvelope(409, 'CONFLICT', 'conflict'));
    const err = await deleteOwnAccount('pw').catch((e) => e);
    expect(err).not.toBeInstanceOf(AccountDeletionRequiresPromotionError);
    expect(err.message).toBe('conflict');
  });

  test('details.reason beats the envelope message', async () => {
    mockApi('delete', url, errorEnvelope(400, 'BAD', 'generic', { reason: 'wrong password' }));
    await expect(deleteOwnAccount('pw')).rejects.toThrow('wrong password');
  });

  test('a plain-text error body becomes the message', async () => {
    mockApi('delete', url, new Response('  boom  ', { status: 500 }));
    await expect(deleteOwnAccount('pw')).rejects.toThrow('boom');
  });

  test('an empty error body falls back to the default message', async () => {
    mockApi('delete', url, new Response('', { status: 500 }));
    await expect(deleteOwnAccount('pw')).rejects.toThrow('Unable to delete account.');
  });

  test('an error envelope with a blank message falls back to the default message', async () => {
    mockApi(
      'delete',
      url,
      new Response(JSON.stringify({ error: { message: '   ' } }), { status: 400 }),
    );
    await expect(deleteOwnAccount('pw')).rejects.toThrow('Unable to delete account.');
  });
});

describe('formatSuggestionAddress', () => {
  test('returns "" for a missing address', () => {
    expect(formatSuggestionAddress(undefined as never)).toBe('');
  });

  test('prefers the full text', () => {
    expect(
      formatSuggestionAddress({
        full: '1 Main St',
        components: [{ kind: 'locality', value: 'X' }],
      }),
    ).toBe('1 Main St');
  });

  test('joins components in display order, first value per kind wins, blanks skipped', () => {
    expect(
      formatSuggestionAddress({
        components: [
          { kind: 'country', value: 'US' },
          { kind: 'locality', value: 'Springfield' },
          { kind: 'locality', value: 'Ignored' },
          { kind: 'name', value: '1 Main St' },
          { kind: 'region', value: '   ' },
          { kind: 'postcode', value: '12345' },
          { kind: 'other', value: 'nope' },
        ],
      } as never),
    ).toBe('1 Main St, Springfield, 12345, US');
  });

  test('an empty object renders an empty line instead of throwing', () => {
    expect(formatSuggestionAddress({})).toBe('');
  });
});
