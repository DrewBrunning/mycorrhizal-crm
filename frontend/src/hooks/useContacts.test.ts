import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type Contact, type ContactsResponse, getContacts } from '../api/contacts';
import { useContacts } from './useContacts';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getContacts).mockReset();
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
});

const contact = (id: number, name: string): Contact => ({ ID: id, firstname: name, lastname: '' });

const page = (contacts: Contact[], next_cursor = ''): ContactsResponse => ({
  contacts,
  next_cursor,
  limit: 25,
  hidden_count: undefined,
});

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

test('loads the first page on mount', async () => {
  vi.mocked(getContacts).mockResolvedValue(page([contact(1, 'Alice')], 'cursor-1'));

  const { result } = renderHook(() => useContacts({ search: 'ali' }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.contacts).toHaveLength(1);
  expect(result.current.nextCursor).toBe('cursor-1');
  expect(getContacts).toHaveBeenCalledTimes(1);
  expect(vi.mocked(getContacts).mock.calls[0][0]).toMatchObject({ search: 'ali' });
  // First page must not carry a cursor.
  expect(vi.mocked(getContacts).mock.calls[0][0].cursor).toBeUndefined();
});

test('loadMore appends the next page and passes the cursor (does not refetch page one) (#556)', async () => {
  vi.mocked(getContacts)
    .mockResolvedValueOnce(page([contact(1, 'A'), contact(2, 'B')], 'cursor-1'))
    .mockResolvedValueOnce(page([contact(3, 'C'), contact(4, 'D')], ''));

  const { result } = renderHook(() => useContacts());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(result.current.contacts.map((c) => c.ID)).toEqual([1, 2, 3, 4]);
  expect(result.current.nextCursor).toBe('');
  // The second call resumes from the cursor rather than re-requesting page one.
  expect(vi.mocked(getContacts).mock.calls[1][0]).toMatchObject({ cursor: 'cursor-1' });
});

test('loadMore surfaces an error and stops loading when the next page fails', async () => {
  vi.mocked(getContacts)
    .mockResolvedValueOnce(page([contact(1, 'A')], 'cursor-1'))
    .mockRejectedValueOnce(new Error('page two boom'));

  const { result } = renderHook(() => useContacts());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(result.current.error).toBe('page two boom');
  expect(result.current.loading).toBe(false);
  // The already-loaded first page is untouched.
  expect(result.current.contacts.map((c) => c.ID)).toEqual([1]);
});

test('a stale search response that settles after a newer query does not clobber the list (#556)', async () => {
  // Simulates typing: "ab" is in flight (slow) when "abc" fires and returns first.
  const slowOld = deferred<ContactsResponse>();
  const fastNew = deferred<ContactsResponse>();
  vi.mocked(getContacts).mockReturnValueOnce(slowOld.promise).mockReturnValueOnce(fastNew.promise);

  const { result, rerender } = renderHook(({ search }) => useContacts({ search }), {
    initialProps: { search: 'ab' },
  });
  await act(async () => {});

  // The query changes before the first request settles.
  rerender({ search: 'abc' });
  await act(async () => {
    fastNew.resolve(page([contact(99, 'abc-match')], ''));
  });
  expect(result.current.contacts.map((c) => c.ID)).toEqual([99]);

  // The stale "ab" response arriving late must be dropped.
  await act(async () => {
    slowOld.resolve(page([contact(1, 'ab-stale'), contact(2, 'ab-stale-2')], 'stale-cursor'));
  });
  expect(result.current.contacts.map((c) => c.ID)).toEqual([99]);
  expect(result.current.nextCursor).toBe('');
});

test('a stale search failure that settles after a newer query does not clobber the list (#556)', async () => {
  // Same overlapping-deferred shape as the success-path guard test above, but
  // the slow "ab" request fails instead of succeeding after "abc" returned.
  const slowOld = deferred<ContactsResponse>();
  const fastNew = deferred<ContactsResponse>();
  vi.mocked(getContacts).mockReturnValueOnce(slowOld.promise).mockReturnValueOnce(fastNew.promise);

  const { result, rerender } = renderHook(({ search }) => useContacts({ search }), {
    initialProps: { search: 'ab' },
  });
  await act(async () => {});

  // The query changes before the first request settles.
  rerender({ search: 'abc' });
  await act(async () => {
    fastNew.resolve(page([contact(99, 'abc-match')], ''));
  });
  expect(result.current.contacts.map((c) => c.ID)).toEqual([99]);
  expect(result.current.error).toBeNull();

  // The stale "ab" failure arriving late must be dropped, not surfaced.
  await act(async () => {
    slowOld.reject(new Error('stale ab failure'));
  });
  expect(result.current.contacts.map((c) => c.ID)).toEqual([99]);
  expect(result.current.error).toBeNull();
});

test('a stale loadMore response that settles after a newer loadMore does not clobber the list (#556)', async () => {
  // Two overlapping "next page" requests: the second is newer and settles
  // first, the first ("stale") settles afterwards and must be dropped.
  const staleNext = deferred<ContactsResponse>();
  const freshNext = deferred<ContactsResponse>();
  vi.mocked(getContacts)
    .mockResolvedValueOnce(page([contact(1, 'A')], 'cursor-1'))
    .mockReturnValueOnce(staleNext.promise)
    .mockReturnValueOnce(freshNext.promise);

  const { result } = renderHook(() => useContacts());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.loadMore();
    result.current.loadMore();
  });

  await act(async () => {
    freshNext.resolve(page([contact(3, 'C')], 'cursor-3'));
  });
  expect(result.current.contacts.map((c) => c.ID)).toEqual([1, 3]);
  expect(result.current.nextCursor).toBe('cursor-3');

  // The stale first loadMore arriving late must be dropped.
  await act(async () => {
    staleNext.resolve(page([contact(2, 'B')], 'stale-cursor'));
  });
  expect(result.current.contacts.map((c) => c.ID)).toEqual([1, 3]);
  expect(result.current.nextCursor).toBe('cursor-3');
});

test('a stale loadMore failure that settles after a newer loadMore does not surface an error (#556)', async () => {
  const staleNext = deferred<ContactsResponse>();
  const freshNext = deferred<ContactsResponse>();
  vi.mocked(getContacts)
    .mockResolvedValueOnce(page([contact(1, 'A')], 'cursor-1'))
    .mockReturnValueOnce(staleNext.promise)
    .mockReturnValueOnce(freshNext.promise);

  const { result } = renderHook(() => useContacts());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.loadMore();
    result.current.loadMore();
  });

  await act(async () => {
    freshNext.resolve(page([contact(3, 'C')], ''));
  });
  expect(result.current.error).toBeNull();

  // The stale first loadMore failing late must be dropped, not surfaced.
  await act(async () => {
    staleNext.reject(new Error('stale loadMore failure'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.contacts.map((c) => c.ID)).toEqual([1, 3]);
});

test('does not fetch when unauthenticated', async () => {
  localStorage.clear();

  const { result } = renderHook(() => useContacts());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContacts).not.toHaveBeenCalled();
  expect(result.current.error).toBe('No authentication token found');
});
