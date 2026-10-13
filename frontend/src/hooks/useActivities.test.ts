import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type Activity, getActivities, getContactActivities } from '../api/activities';
import { useActivities } from './useActivities';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

vi.mock('../api/activities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/activities')>();
  return { ...actual, getActivities: vi.fn(), getContactActivities: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getActivities).mockReset();
  vi.mocked(getContactActivities).mockReset();
});

function activity(id: number, title: string): Activity {
  return {
    ID: id,
    title,
    date: '2026-08-12T10:00:00Z',
    CreatedAt: '2026-08-12T10:00:00Z',
    UpdatedAt: '2026-08-12T10:00:00Z',
  };
}

test('initial fetch populates activities and the next cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities).mockResolvedValue({
    activities: [activity(1, 'call with alice'), activity(2, 'coffee with bob')],
    next_cursor: 'cursor-1',
    limit: 25,
  });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getActivities).toHaveBeenCalledWith({
    limit: undefined,
    includeContacts: undefined,
    search: undefined,
    fromDate: undefined,
    toDate: undefined,
  });
  expect(result.current.activities.map((a) => a.ID)).toEqual([1, 2]);
  expect(result.current.nextCursor).toBe('cursor-1');
  expect(result.current.limit).toBe(25);
  expect(result.current.error).toBeNull();
});

test('loadMore appends rows and follows the next cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities)
    .mockResolvedValueOnce({
      activities: [activity(1, 'page one')],
      next_cursor: 'cursor-1',
      limit: 25,
    })
    .mockResolvedValueOnce({
      activities: [activity(2, 'page two')],
      next_cursor: '',
      limit: 25,
    });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.activities).toHaveLength(1);

  await act(async () => {
    await result.current.loadMore();
  });

  expect(getActivities).toHaveBeenLastCalledWith({
    cursor: 'cursor-1',
    limit: undefined,
    includeContacts: undefined,
    search: undefined,
    fromDate: undefined,
    toDate: undefined,
  });
  expect(result.current.activities.map((a) => a.ID)).toEqual([1, 2]);
  expect(result.current.nextCursor).toBe('');
});

test('loadMore is a no-op when there is no next cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities).mockResolvedValue({
    activities: [activity(1, 'only page')],
    next_cursor: '',
    limit: 25,
  });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(getActivities).toHaveBeenCalledTimes(1);
});

test('refetch replaces the list', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities)
    .mockResolvedValueOnce({ activities: [activity(1, 'one')], next_cursor: '', limit: 25 })
    .mockResolvedValueOnce({ activities: [activity(2, 'two')], next_cursor: '', limit: 25 });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refetch();
  });

  expect(getActivities).toHaveBeenCalledTimes(2);
  expect(result.current.activities.map((a) => a.ID)).toEqual([2]);
});

test('uses getContactActivities when a contactId is given', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getContactActivities).mockResolvedValue({ activities: [activity(1, 'contact one')] });

  const { result } = renderHook(() => useActivities({}, 'contact-7'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactActivities).toHaveBeenCalledWith('contact-7');
  expect(getActivities).not.toHaveBeenCalled();
  expect(result.current.activities.map((a) => a.ID)).toEqual([1]);
  expect(result.current.nextCursor).toBe('');
});

test('sets an auth error when no token exists', async () => {
  localStorage.clear();

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('No authentication token found');
  expect(result.current.activities).toEqual([]);
  expect(getActivities).not.toHaveBeenCalled();
});

test('sets error when the fetch fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.activities).toEqual([]);
});

test('sets error when loading more fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities)
    .mockResolvedValueOnce({ activities: [activity(1, 'one')], next_cursor: 'cursor-1', limit: 25 })
    .mockRejectedValueOnce(new Error('boom'));

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(result.current.error).toBe('boom');
  expect(result.current.activities).toHaveLength(1);
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

type Page = Awaited<ReturnType<typeof getActivities>>;

function signIn() {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
}

test('starts empty with the requested page size (or 25) before the first response', () => {
  signIn();
  vi.mocked(getActivities).mockReturnValue(new Promise(() => {}));

  const plain = renderHook(() => useActivities());
  expect(plain.result.current.activities).toEqual([]);
  expect(plain.result.current.nextCursor).toBe('');
  expect(plain.result.current.limit).toBe(25);
  expect(plain.result.current.error).toBeNull();

  const sized = renderHook(() => useActivities({ limit: 7 }));
  expect(sized.result.current.limit).toBe(7);
});

test('passes the filter params to the first-page fetch', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValue({ activities: [], next_cursor: '', limit: 10 });

  const { result } = renderHook(() =>
    useActivities({
      limit: 10,
      includeContacts: true,
      search: 'lunch',
      fromDate: '2026-01-01',
      toDate: '2026-02-01',
    }),
  );
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getActivities).toHaveBeenCalledWith({
    limit: 10,
    includeContacts: true,
    search: 'lunch',
    fromDate: '2026-01-01',
    toDate: '2026-02-01',
  });
});

test('limit prefers the server value, then the requested size, then 25', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValueOnce({ activities: [], next_cursor: '', limit: 50 });
  const a = renderHook(() => useActivities({ limit: 10 }));
  await waitFor(() => expect(a.result.current.loading).toBe(false));
  expect(a.result.current.limit).toBe(50);

  vi.mocked(getActivities).mockResolvedValueOnce({
    activities: [],
    next_cursor: '',
  } as unknown as Page);
  const b = renderHook(() => useActivities({ limit: 10 }));
  await waitFor(() => expect(b.result.current.loading).toBe(false));
  expect(b.result.current.limit).toBe(10);

  vi.mocked(getActivities).mockResolvedValueOnce({
    activities: [],
    next_cursor: '',
  } as unknown as Page);
  const c = renderHook(() => useActivities());
  await waitFor(() => expect(c.result.current.loading).toBe(false));
  expect(c.result.current.limit).toBe(25);
});

test('tolerates a response with no activities and no cursor', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValue({} as Page);

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.activities).toEqual([]);
  expect(result.current.nextCursor).toBe('');
});

test('the contact-scoped list sizes the page from the requested limit, else the row count, else 25', async () => {
  signIn();
  const rows = [activity(1, 'a'), activity(2, 'b'), activity(3, 'c')];

  vi.mocked(getContactActivities).mockResolvedValueOnce({ activities: rows });
  const counted = renderHook(() => useActivities({}, 9));
  await waitFor(() => expect(counted.result.current.loading).toBe(false));
  expect(counted.result.current.limit).toBe(3);

  vi.mocked(getContactActivities).mockResolvedValueOnce({ activities: rows });
  const sized = renderHook(() => useActivities({ limit: 12 }, 9));
  await waitFor(() => expect(sized.result.current.loading).toBe(false));
  expect(sized.result.current.limit).toBe(12);

  vi.mocked(getContactActivities).mockResolvedValueOnce({} as never);
  const empty = renderHook(() => useActivities({}, 9));
  await waitFor(() => expect(empty.result.current.loading).toBe(false));
  expect(empty.result.current.limit).toBe(25);
  expect(empty.result.current.activities).toEqual([]);
  expect(empty.result.current.error).toBeNull();
});

test('switching to a contact-scoped list drops the previous cursor', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValue({
    activities: [activity(1, 'one')],
    next_cursor: 'cursor-1',
    limit: 25,
  });
  vi.mocked(getContactActivities).mockResolvedValue({ activities: [activity(2, 'two')] });

  const { result, rerender } = renderHook(({ id }) => useActivities({}, id), {
    initialProps: { id: undefined as string | undefined },
  });
  await waitFor(() => expect(result.current.nextCursor).toBe('cursor-1'));

  rerender({ id: 'c-1' });
  await waitFor(() => expect(result.current.activities.map((a) => a.ID)).toEqual([2]));
  expect(result.current.nextCursor).toBe('');
});

test('loading is true while the first page is in flight and an old error is cleared by a refetch', async () => {
  signIn();
  vi.mocked(getActivities).mockRejectedValueOnce(new Error('boom'));
  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.error).toBe('boom'));
  expect(console.error).toHaveBeenCalledWith('[fetching activities] Error:', expect.anything());

  const d = deferred<Page>();
  vi.mocked(getActivities).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve({ activities: [], next_cursor: '', limit: 25 });
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
});

test('a superseded first-page response is ignored (list and contact variants)', async () => {
  signIn();
  for (const scoped of [false, true]) {
    const api = (scoped ? getContactActivities : getActivities) as unknown as ReturnType<
      typeof vi.fn
    >;
    const stale = deferred<never>();
    const fresh = deferred<never>();
    api.mockReset();
    api.mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);

    const { result } = renderHook(() => useActivities({}, scoped ? 5 : undefined));
    act(() => {
      void result.current.refetch();
    });
    await waitFor(() => expect(api).toHaveBeenCalledTimes(2));

    await act(async () => {
      fresh.resolve({ activities: [activity(2, 'fresh')], next_cursor: 'n', limit: 5 } as never);
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      stale.resolve({ activities: [activity(1, 'stale')], next_cursor: 's', limit: 9 } as never);
    });

    expect(result.current.activities.map((a) => a.ID)).toEqual([2]);
    expect(result.current.limit).not.toBe(9);
    expect(result.current.loading).toBe(false);
  }
});

test('a superseded first-page failure neither errors nor ends loading', async () => {
  signIn();
  const stale = deferred<never>();
  const fresh = deferred<never>();
  vi.mocked(getActivities).mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
  const { result } = renderHook(() => useActivities());
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(getActivities).toHaveBeenCalledTimes(2));

  await act(async () => {
    stale.reject(new Error('old failure'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.loading).toBe(true);

  await act(async () => {
    fresh.resolve({ activities: [], next_cursor: '', limit: 25 } as never);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.error).toBeNull();
});

test('a superseded first-page response that lands last does not end loading early', async () => {
  signIn();
  const stale = deferred<never>();
  const fresh = deferred<never>();
  vi.mocked(getActivities).mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
  const { result } = renderHook(() => useActivities());
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(getActivities).toHaveBeenCalledTimes(2));

  await act(async () => {
    stale.resolve({ activities: [], next_cursor: '', limit: 25 } as never);
  });
  expect(result.current.loading).toBe(true);
});

test('loadMore sends the cursor with the filters, toggles loading, and appends the next cursor', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValueOnce({
    activities: [activity(1, 'one')],
    next_cursor: 'c1',
    limit: 25,
  });
  const { result } = renderHook(() =>
    useActivities({ limit: 5, includeContacts: true, search: 's', fromDate: 'f', toDate: 't' }),
  );
  await waitFor(() => expect(result.current.nextCursor).toBe('c1'));

  const d = deferred<Page>();
  vi.mocked(getActivities).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.loadMore();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getActivities).toHaveBeenLastCalledWith({
    cursor: 'c1',
    limit: 5,
    includeContacts: true,
    search: 's',
    fromDate: 'f',
    toDate: 't',
  });
  await act(async () => {
    d.resolve({ activities: [activity(2, 'two')], next_cursor: '', limit: 5 });
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.activities.map((a) => a.ID)).toEqual([1, 2]);
  expect(result.current.nextCursor).toBe('');
});

test('loadMore tolerates a page with no activities and clears a previous error while loading', async () => {
  signIn();
  vi.mocked(getActivities)
    .mockResolvedValueOnce({ activities: [activity(1, 'one')], next_cursor: 'c1', limit: 25 })
    .mockRejectedValueOnce(new Error('first fail'))
    .mockResolvedValueOnce({} as Page);
  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.nextCursor).toBe('c1'));

  await act(async () => {
    await result.current.loadMore();
  });
  expect(result.current.error).toBe('first fail');
  expect(console.error).toHaveBeenCalledWith('[loading more activities] Error:', expect.anything());

  await act(async () => {
    await result.current.loadMore();
  });
  expect(result.current.error).toBeNull();
  expect(result.current.activities).toHaveLength(1);
  expect(result.current.nextCursor).toBe('');
});

test('a superseded loadMore response or failure is ignored and does not end loading', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValueOnce({
    activities: [activity(1, 'one')],
    next_cursor: 'c1',
    limit: 25,
  });
  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.nextCursor).toBe('c1'));

  const staleMore = deferred<never>();
  const refetch = deferred<never>();
  vi.mocked(getActivities)
    .mockReturnValueOnce(staleMore.promise)
    .mockReturnValueOnce(refetch.promise);
  act(() => {
    void result.current.loadMore();
  });
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(getActivities).toHaveBeenCalledTimes(3));

  await act(async () => {
    staleMore.resolve({
      activities: [activity(9, 'late')],
      next_cursor: 'zzz',
      limit: 25,
    } as never);
  });
  expect(result.current.activities.map((a) => a.ID)).toEqual([1]);
  expect(result.current.nextCursor).toBe('c1');
  expect(result.current.loading).toBe(true);

  await act(async () => {
    refetch.resolve({ activities: [activity(2, 'fresh')], next_cursor: '', limit: 25 } as never);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.activities.map((a) => a.ID)).toEqual([2]);
});

test('a superseded loadMore failure is ignored', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValueOnce({
    activities: [activity(1, 'one')],
    next_cursor: 'c1',
    limit: 25,
  });
  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.nextCursor).toBe('c1'));

  const staleMore = deferred<never>();
  const refetch = deferred<never>();
  vi.mocked(getActivities)
    .mockReturnValueOnce(staleMore.promise)
    .mockReturnValueOnce(refetch.promise);
  act(() => {
    void result.current.loadMore();
  });
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(getActivities).toHaveBeenCalledTimes(3));

  await act(async () => {
    staleMore.reject(new Error('late failure'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.loading).toBe(true);
});

test('refetch and loadMore follow filter changes', async () => {
  signIn();
  vi.mocked(getActivities).mockResolvedValue({
    activities: [],
    next_cursor: 'c',
    limit: 25,
  });
  const { result, rerender } = renderHook(({ q }) => useActivities({ search: q }), {
    initialProps: { q: 'a' },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ q: 'b' });
  await waitFor(() =>
    expect(getActivities).toHaveBeenLastCalledWith(expect.objectContaining({ search: 'b' })),
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    await result.current.loadMore();
  });
  expect(getActivities).toHaveBeenLastCalledWith(
    expect.objectContaining({ cursor: 'c', search: 'b' }),
  );
});
