import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import type { ShareGrantPage } from '../api/shareGrants';
import { useShareGrantList } from './useShareGrantList';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

type Row = { id: string; label: string };

function page(rows: Row[], next = ''): ShareGrantPage<Row> {
  return { share_grants: rows, next_cursor: next, limit: 2 };
}

function deferred<T>() {
  let resolve: (v: T) => void = () => undefined;
  let reject: (e: unknown) => void = () => undefined;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

test('starts empty and loading until the first refresh', () => {
  const { result } = renderHook(() => useShareGrantList<Row>(vi.fn()));
  expect(result.current.items).toEqual([]);
  expect(result.current.loading).toBe(true);
  expect(result.current.error).toBe('');
  expect(result.current.nextCursor).toBe('');
});

test('refresh loads the first page and exposes the cursor for Load more', async () => {
  const fetchPage = vi.fn().mockResolvedValue(page([{ id: 'a', label: 'A' }], 'c1'));
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));

  await act(() => result.current.refresh());

  expect(fetchPage).toHaveBeenCalledWith({});
  expect(result.current.items).toEqual([{ id: 'a', label: 'A' }]);
  expect(result.current.nextCursor).toBe('c1');
  expect(result.current.loading).toBe(false);
});

test('loadMore appends the next page using the cursor and ends when the cursor is empty', async () => {
  const fetchPage = vi
    .fn()
    .mockResolvedValueOnce(page([{ id: 'a', label: 'A' }], 'c1'))
    .mockResolvedValueOnce(page([{ id: 'b', label: 'B' }], ''));
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));

  await act(() => result.current.refresh());
  await act(() => result.current.loadMore());

  expect(fetchPage).toHaveBeenLastCalledWith({ cursor: 'c1' });
  expect(result.current.items.map((r) => r.id)).toEqual(['a', 'b']);
  expect(result.current.nextCursor).toBe('');

  // No cursor left: nothing is fetched.
  await act(() => result.current.loadMore());
  expect(fetchPage).toHaveBeenCalledTimes(2);
});

test('a failed refresh shows the translated reason-coded message and stops loading', async () => {
  const fetchPage = vi
    .fn()
    .mockRejectedValue(
      new ApiError('raw', 'FORBIDDEN', 403, undefined, undefined, 'share_grant_not_found'),
    );
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));

  await act(() => result.current.refresh());

  expect(result.current.error).not.toBe('');
  expect(result.current.error).not.toBe('raw');
  expect(result.current.loading).toBe(false);
  expect(result.current.items).toEqual([]);
});

test('a failed loadMore keeps the rows already shown and reports the error', async () => {
  const fetchPage = vi
    .fn()
    .mockResolvedValueOnce(page([{ id: 'a', label: 'A' }], 'c1'))
    .mockRejectedValueOnce(new ApiError('Server exploded', 'INTERNAL_ERROR', 500));
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));

  await act(() => result.current.refresh());
  await act(() => result.current.loadMore());

  expect(result.current.items).toEqual([{ id: 'a', label: 'A' }]);
  expect(result.current.error).toContain('Server exploded');
  expect(result.current.nextCursor).toBe('c1');
  expect(result.current.loading).toBe(false);
});

test('a slow earlier refresh cannot overwrite a newer one (stale-response guard)', async () => {
  const slow = deferred<ShareGrantPage<Row>>();
  const fast = deferred<ShareGrantPage<Row>>();
  const fetchPage = vi.fn().mockReturnValueOnce(slow.promise).mockReturnValueOnce(fast.promise);
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));

  let first: Promise<void> = Promise.resolve();
  let second: Promise<void> = Promise.resolve();
  act(() => {
    first = result.current.refresh();
    second = result.current.refresh();
  });
  await act(async () => {
    fast.resolve(page([{ id: 'new', label: 'new' }]));
    await second;
  });
  expect(result.current.items).toEqual([{ id: 'new', label: 'new' }]);
  expect(result.current.loading).toBe(false);

  await act(async () => {
    slow.resolve(page([{ id: 'old', label: 'old' }], 'stale-cursor'));
    await first;
  });
  expect(result.current.items).toEqual([{ id: 'new', label: 'new' }]);
  expect(result.current.nextCursor).toBe('');
});

test('a stale failure is ignored too', async () => {
  const slow = deferred<ShareGrantPage<Row>>();
  const fetchPage = vi
    .fn()
    .mockReturnValueOnce(slow.promise)
    .mockResolvedValueOnce(page([{ id: 'ok', label: 'ok' }]));
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));

  let first: Promise<void> = Promise.resolve();
  act(() => {
    first = result.current.refresh();
  });
  await act(() => result.current.refresh());
  await act(async () => {
    slow.reject(new ApiError('late failure', 'INTERNAL_ERROR', 500));
    await first;
  });

  expect(result.current.error).toBe('');
  expect(result.current.items).toEqual([{ id: 'ok', label: 'ok' }]);
});

test('replace swaps the matching row in place and remove drops it', async () => {
  const fetchPage = vi.fn().mockResolvedValue(
    page([
      { id: 'a', label: 'A' },
      { id: 'b', label: 'B' },
    ]),
  );
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));
  await act(() => result.current.refresh());

  act(() => result.current.replace({ id: 'b', label: 'B2' }));
  expect(result.current.items).toEqual([
    { id: 'a', label: 'A' },
    { id: 'b', label: 'B2' },
  ]);
  act(() => result.current.replace({ id: 'zzz', label: 'ignored' }));
  expect(result.current.items).toHaveLength(2);

  act(() => result.current.remove('a'));
  expect(result.current.items).toEqual([{ id: 'b', label: 'B2' }]);
  await waitFor(() => expect(result.current.loading).toBe(false));
});

// A Load more that was overtaken by a refresh must not append its page (or its
// error) onto the refreshed list.
test.each([
  [
    'page',
    (d: ReturnType<typeof deferred<ShareGrantPage<Row>>>) =>
      d.resolve(page([{ id: 'late', label: 'late' }], 'later')),
  ],
  [
    'failure',
    (d: ReturnType<typeof deferred<ShareGrantPage<Row>>>) =>
      d.reject(new ApiError('late failure', 'INTERNAL_ERROR', 500)),
  ],
])('a superseded Load more %s is ignored', async (_name, settle) => {
  const more = deferred<ShareGrantPage<Row>>();
  const fetchPage = vi
    .fn()
    .mockResolvedValueOnce(page([{ id: 'a', label: 'A' }], 'c1'))
    .mockReturnValueOnce(more.promise)
    .mockResolvedValueOnce(page([{ id: 'fresh', label: 'fresh' }], ''));
  const { result } = renderHook(() => useShareGrantList<Row>(fetchPage));
  await act(() => result.current.refresh());

  let pending: Promise<void> = Promise.resolve();
  act(() => {
    pending = result.current.loadMore();
  });
  await act(() => result.current.refresh());
  await act(async () => {
    settle(more);
    await pending;
  });

  expect(result.current.items).toEqual([{ id: 'fresh', label: 'fresh' }]);
  expect(result.current.nextCursor).toBe('');
  expect(result.current.error).toBe('');
  expect(result.current.loading).toBe(false);
});
