import { afterEach, describe, expect, test, vi } from 'vitest';
import { ApiError } from './client';
import {
  emptyTrash,
  getTrashedContacts,
  purgeTrashedContact,
  restoreTrashedContact,
} from './trash';

afterEach(() => {
  vi.unstubAllGlobals();
});

const summary = {
  id: 7,
  uid: 'uid-7',
  firstname: 'Ada',
  lastname: 'Lovelace',
  nickname: '',
  fn: 'Ada Lovelace',
  primary_email: '',
  primary_phone: '',
  birthday: '',
  org: '',
  photo: '',
  archived: false,
  is_favorite: false,
  revision: 3,
};

function stubFetch(response: unknown) {
  const fetchMock = vi.fn(async () => response);
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('getTrashedContacts', () => {
  test('requests the page and maps rows', async () => {
    const fetchMock = stubFetch({
      ok: true,
      json: async () => ({
        contacts: [{ contact: summary, deleted_at: '2026-10-01T00:00:00Z', purge_at: null }],
        total: 1,
        page: 2,
        limit: 10,
        retention_days: 0,
      }),
    });
    const out = await getTrashedContacts({ page: 2, limit: 10 });
    expect(String((fetchMock.mock.calls[0] as unknown[])[0])).toContain(
      '/trash/contacts?page=2&limit=10',
    );
    expect(out.contacts[0].contact.firstname).toBe('Ada');
    expect(out.contacts[0].purge_at).toBeNull();
    expect(out.retention_days).toBe(0);
  });

  test('tolerates an absent contacts array', async () => {
    stubFetch({
      ok: true,
      json: async () => ({ total: 0, page: 1, limit: 25, retention_days: 30 }),
    });
    expect((await getTrashedContacts()).contacts).toEqual([]);
  });

  test('throws an ApiError on failure', async () => {
    stubFetch({
      ok: false,
      status: 500,
      statusText: 'x',
      json: async () => ({ error: { code: 'DATABASE_ERROR', message: 'boom' } }),
    });
    await expect(getTrashedContacts()).rejects.toBeInstanceOf(ApiError);
  });
});

describe('restoreTrashedContact', () => {
  test('POSTs to the restore route and returns skipped rows', async () => {
    const fetchMock = stubFetch({
      ok: true,
      json: async () => ({
        contact: summary,
        skipped: [{ table: 'relationship_edges', reason: 'x', count: 2 }],
      }),
    });
    const out = await restoreTrashedContact(7);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toContain('/trash/contacts/7/restore');
    expect(init.method).toBe('POST');
    expect(out.skipped[0].count).toBe(2);
  });

  test('defaults skipped to [] when absent', async () => {
    stubFetch({ ok: true, json: async () => ({ contact: summary }) });
    expect((await restoreTrashedContact(7)).skipped).toEqual([]);
  });

  test('surfaces a 409 as an ApiError with status 409', async () => {
    stubFetch({
      ok: false,
      status: 409,
      statusText: 'Conflict',
      json: async () => ({ error: { code: 'CONFLICT', message: 'uid in use' } }),
    });
    await expect(restoreTrashedContact(7)).rejects.toMatchObject({ status: 409 });
  });
});

describe('purge and empty', () => {
  test('purgeTrashedContact DELETEs the item route', async () => {
    const fetchMock = stubFetch({ ok: true, json: async () => ({ message: 'ok', purged: 1 }) });
    await purgeTrashedContact(7);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toMatch(/\/trash\/contacts\/7$/);
    expect(init.method).toBe('DELETE');
  });

  test('emptyTrash DELETEs the collection and returns the count', async () => {
    const fetchMock = stubFetch({ ok: true, json: async () => ({ message: 'ok', purged: 4 }) });
    expect(await emptyTrash()).toBe(4);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toMatch(/\/trash\/contacts$/);
    expect(init.method).toBe('DELETE');
  });
});
