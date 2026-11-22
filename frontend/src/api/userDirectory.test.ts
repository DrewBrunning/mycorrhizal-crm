import { expect, test } from 'vitest';
import '../i18n/config';
import { mockApi, setupMswServer } from '../test/mswServer';
import { getUserDirectory } from './contactShares';

setupMswServer();

test('getUserDirectory sends no q parameter without a query (all mode / initial load)', async () => {
  const calls = mockApi('get', '/users/directory', { users: [{ id: 2, username: 'bob' }] });
  expect(await getUserDirectory()).toEqual([{ id: 2, username: 'bob' }]);
  expect(calls[0].search.has('q')).toBe(false);
});

test('getUserDirectory sends the trimmed query as q', async () => {
  const calls = mockApi('get', '/users/directory', { users: [] });
  await getUserDirectory('  bob ');
  expect(calls[0].search.get('q')).toBe('bob');
});

test('getUserDirectory omits q for a blank query', async () => {
  const calls = mockApi('get', '/users/directory', { users: [] });
  await getUserDirectory('   ');
  expect(calls[0].search.has('q')).toBe(false);
});
