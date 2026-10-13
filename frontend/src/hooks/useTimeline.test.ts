import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  getTimeline,
  TIMELINE_BUCKETS,
  TIMELINE_TYPES,
  type TimelineItem,
  type TimelineResponse,
} from '../api/timeline';
import { useTimeline } from './useTimeline';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/timeline', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/timeline')>();
  return { ...actual, getTimeline: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getTimeline).mockReset();
});

function noteItem(id: string, content: string): TimelineItem {
  return {
    type: 'note',
    id,
    date: '2026-08-12T10:00:00Z',
    data: {
      ID: Number(id),
      content,
      date: '2026-08-12T10:00:00Z',
      CreatedAt: '2026-08-12T10:00:00Z',
      UpdatedAt: '2026-08-12T10:00:00Z',
    },
  };
}

function page(items: TimelineItem[], nextCursor: string): TimelineResponse {
  return { items, next_cursor: nextCursor, limit: 25 };
}

test('a load-more that resolves after a filter change is discarded, not appended', async () => {
  let resolveLoadMore!: (value: TimelineResponse) => void;
  let resolveRefresh!: (value: TimelineResponse) => void;

  // First call is the initial refresh; we drive the rest by hand.
  vi.mocked(getTimeline)
    .mockResolvedValueOnce(page([noteItem('1', 'page one')], 'cursor-1'))
    // The load-more (cursor=1) hangs until we resolve it.
    .mockImplementationOnce(
      () =>
        new Promise((res) => {
          resolveLoadMore = res;
        }),
    )
    // The refresh after the filter change hangs until we resolve it.
    .mockImplementationOnce(
      () =>
        new Promise((res) => {
          resolveRefresh = res;
        }),
    );

  const { result } = renderHook(() => useTimeline(42));

  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.items.map((i) => i.id)).toEqual(['1']);

  // Start a load-more against cursor-1 (it hangs in flight).
  let loadMorePromise: Promise<void>;
  act(() => {
    loadMorePromise = result.current.loadMore();
  });

  // Change the filter, then trigger the refresh the dialog's effect would --
  // this supersedes the in-flight load-more.
  await act(async () => {
    result.current.setTypes(['gift']);
  });
  let refreshPromise: Promise<void>;
  act(() => {
    refreshPromise = result.current.refresh();
  });
  await act(async () => {
    resolveRefresh(page([noteItem('2', 'gift only')], ''));
    await refreshPromise;
  });

  // Now the stale load-more resolves -- it must be discarded.
  await act(async () => {
    resolveLoadMore(page([noteItem('3', 'stale page two')], ''));
    await loadMorePromise;
  });

  expect(result.current.items.map((i) => i.id)).toEqual(['2']);
  expect(result.current.nextCursor).toBe('');
});

test('switching contacts clears the previous contact page', async () => {
  vi.mocked(getTimeline)
    .mockResolvedValueOnce(page([noteItem('1', 'contact A note')], ''))
    .mockResolvedValueOnce(page([noteItem('2', 'contact B note')], ''));

  const { result, rerender } = renderHook(({ id }) => useTimeline(id), {
    initialProps: { id: 1 as number | undefined },
  });

  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.items.map((i) => i.id)).toEqual(['1']);

  // Navigate to contact 2: the page should clear before any refetch, so
  // contact A's rows never render under contact B.
  await act(async () => {
    rerender({ id: 2 });
  });

  expect(result.current.items).toEqual([]);
  expect(result.current.nextCursor).toBe('');

  await act(async () => {
    await result.current.refresh();
  });
  await waitFor(() => expect(result.current.items.map((i) => i.id)).toEqual(['2']));
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

test('starts idle with the full type set (a copy) and the all bucket', () => {
  const { result } = renderHook(() => useTimeline(1));
  expect(result.current.items).toEqual([]);
  expect(result.current.nextCursor).toBe('');
  expect(result.current.loading).toBe(false);
  expect(result.current.loadingMore).toBe(false);
  expect(result.current.error).toBeNull();
  expect(result.current.types).toEqual([...TIMELINE_TYPES]);
  expect(result.current.types).not.toBe(TIMELINE_TYPES);
  expect(result.current.bucket).toBe('all');
});

test('refresh without a contact does nothing', async () => {
  const { result } = renderHook(() => useTimeline(undefined));
  await act(async () => {
    await result.current.refresh();
    await result.current.loadMore();
  });
  expect(getTimeline).not.toHaveBeenCalled();
  expect(result.current.loading).toBe(false);
});

test('refresh sends the contact, types and bucket, and toggles loading around the call', async () => {
  const d = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValue(d.promise);
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    result.current.setTypes(['gift']);
    result.current.setBucket('all');
  });
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getTimeline).toHaveBeenCalledWith({ contactId: 5, types: ['gift'], bucket: 'all' });
  await act(async () => {
    d.resolve(page([noteItem('1', 'a')], 'c1'));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.items.map((i) => i.id)).toEqual(['1']);
  expect(result.current.nextCursor).toBe('c1');
});

test('refresh uses the latest bucket and types after they change', async () => {
  vi.mocked(getTimeline).mockResolvedValue(page([], ''));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    result.current.setBucket('all');
    result.current.setTypes(['note']);
  });
  await act(async () => {
    await result.current.refresh();
  });
  expect(getTimeline).toHaveBeenLastCalledWith({ contactId: 5, types: ['note'], bucket: 'all' });
  const other = otherBucket();
  await act(async () => {
    result.current.setBucket(other);
  });
  await act(async () => {
    await result.current.refresh();
  });
  expect(getTimeline).toHaveBeenLastCalledWith({ contactId: 5, types: ['note'], bucket: other });
});

function otherBucket() {
  return TIMELINE_BUCKETS.find((b) => b !== 'all') as (typeof TIMELINE_BUCKETS)[number];
}

test('refresh tolerates a response with no items or cursor', async () => {
  vi.mocked(getTimeline).mockResolvedValue({} as TimelineResponse);
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.items).toEqual([]);
  expect(result.current.nextCursor).toBe('');
});

test('a failed refresh reports the error and logs the operation; the next refresh clears it', async () => {
  vi.mocked(getTimeline).mockRejectedValueOnce(new Error('down'));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('down');
  expect(result.current.loading).toBe(false);
  expect(console.error).toHaveBeenCalledWith('[fetching timeline] Error:', expect.anything());

  const d = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve(page([], ''));
  });
});

test('a refresh supersedes an in-flight load-more and ends its spinner', async () => {
  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], 'c1'));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  const more = deferred<TimelineResponse>();
  const fresh = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(more.promise).mockReturnValueOnce(fresh.promise);
  act(() => {
    void result.current.loadMore();
  });
  await waitFor(() => expect(result.current.loadingMore).toBe(true));
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.loadingMore).toBe(false);
  await act(async () => {
    fresh.resolve(page([], ''));
  });
});

test('a superseded refresh response, failure and completion are all ignored', async () => {
  const stale = deferred<TimelineResponse>();
  const fresh = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
  const { result } = renderHook(() => useTimeline(5));
  act(() => {
    void result.current.refresh();
  });
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(getTimeline).toHaveBeenCalledTimes(2));

  await act(async () => {
    stale.reject(new Error('old'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.loading).toBe(true);

  await act(async () => {
    fresh.resolve(page([noteItem('2', 'fresh')], ''));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.items.map((i) => i.id)).toEqual(['2']);
});

test('a superseded refresh success that lands last does not overwrite or end loading', async () => {
  const stale = deferred<TimelineResponse>();
  const fresh = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
  const { result } = renderHook(() => useTimeline(5));
  act(() => {
    void result.current.refresh();
  });
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(getTimeline).toHaveBeenCalledTimes(2));
  await act(async () => {
    stale.resolve(page([noteItem('1', 'stale')], 'x'));
  });
  expect(result.current.items).toEqual([]);
  expect(result.current.loading).toBe(true);
});

test('loadMore needs a contact and a cursor, then sends the cursor and appends', async () => {
  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], ''));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  await act(async () => {
    await result.current.loadMore(); // no cursor
  });
  expect(getTimeline).toHaveBeenCalledTimes(1);

  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], 'c1'));
  await act(async () => {
    await result.current.refresh();
  });
  const d = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.loadMore();
  });
  await waitFor(() => expect(result.current.loadingMore).toBe(true));
  expect(getTimeline).toHaveBeenLastCalledWith({
    contactId: 5,
    types: [...TIMELINE_TYPES],
    bucket: 'all',
    cursor: 'c1',
  });
  await act(async () => {
    d.resolve(page([noteItem('2', 'b')], 'c2'));
  });
  await waitFor(() => expect(result.current.loadingMore).toBe(false));
  expect(result.current.items.map((i) => i.id)).toEqual(['1', '2']);
  expect(result.current.nextCursor).toBe('c2');
});

test('loadMore tolerates an empty page, reports failures, and clears the error next time', async () => {
  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], 'c1'));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  vi.mocked(getTimeline).mockRejectedValueOnce(new Error('nope'));
  await act(async () => {
    await result.current.loadMore();
  });
  expect(result.current.error).toBe('nope');
  expect(result.current.loadingMore).toBe(false);
  expect(console.error).toHaveBeenCalledWith('[fetching more timeline] Error:', expect.anything());

  vi.mocked(getTimeline).mockResolvedValueOnce({} as TimelineResponse);
  await act(async () => {
    await result.current.loadMore();
  });
  expect(result.current.error).toBeNull();
  expect(result.current.items).toHaveLength(1);
  expect(result.current.nextCursor).toBe('');
});

test('a superseded loadMore (response, failure, completion) is ignored', async () => {
  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], 'c1'));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  const more = deferred<TimelineResponse>();
  const fresh = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(more.promise).mockReturnValueOnce(fresh.promise);
  act(() => {
    void result.current.loadMore();
  });
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(getTimeline).toHaveBeenCalledTimes(3));
  await act(async () => {
    more.reject(new Error('late'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.loading).toBe(true);
  await act(async () => {
    fresh.resolve(page([noteItem('2', 'b')], ''));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.items.map((i) => i.id)).toEqual(['2']);
});

test('a superseded loadMore success that lands last neither appends nor ends the spinner', async () => {
  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], 'c1'));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  const first = deferred<TimelineResponse>();
  const second = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  act(() => {
    void result.current.loadMore();
  });
  act(() => {
    void result.current.loadMore();
  });
  await waitFor(() => expect(getTimeline).toHaveBeenCalledTimes(3));
  await act(async () => {
    first.resolve(page([noteItem('9', 'dup')], 'zz'));
  });
  expect(result.current.items.map((i) => i.id)).toEqual(['1']);
  expect(result.current.loadingMore).toBe(true);
});

test('switching contacts clears flags and retires in-flight requests', async () => {
  const inflight = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(inflight.promise);
  const { result, rerender } = renderHook(({ id }) => useTimeline(id), {
    initialProps: { id: 1 as number | undefined },
  });
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));

  rerender({ id: 2 });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.loadingMore).toBe(false);
  expect(result.current.error).toBeNull();

  await act(async () => {
    inflight.resolve(page([noteItem('1', 'A')], 'x'));
  });
  expect(result.current.items).toEqual([]);
  expect(result.current.nextCursor).toBe('');
});

test('switching contacts clears a lingering error, cursor, and load-more spinner', async () => {
  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], 'c1'));
  const { result, rerender } = renderHook(({ id }) => useTimeline(id), {
    initialProps: { id: 1 as number | undefined },
  });
  await act(async () => {
    await result.current.refresh();
  });
  const more = deferred<TimelineResponse>();
  vi.mocked(getTimeline).mockReturnValueOnce(more.promise);
  act(() => {
    void result.current.loadMore();
  });
  await waitFor(() => expect(result.current.loadingMore).toBe(true));
  rerender({ id: 2 });
  await waitFor(() => expect(result.current.loadingMore).toBe(false));
  expect(result.current.nextCursor).toBe('');

  vi.mocked(getTimeline).mockRejectedValueOnce(new Error('bad'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('bad');
  rerender({ id: 3 });
  await waitFor(() => expect(result.current.error).toBeNull());
});

test('interleaved loadMore and refresh requests never share an id: an old loadMore stays stale', async () => {
  vi.mocked(getTimeline).mockResolvedValueOnce(page([noteItem('1', 'a')], 'c1'));
  const { result } = renderHook(() => useTimeline(5));
  await act(async () => {
    await result.current.refresh();
  });
  const firstMore = deferred<TimelineResponse>();
  const refresh = deferred<TimelineResponse>();
  const secondMore = deferred<TimelineResponse>();
  vi.mocked(getTimeline)
    .mockReturnValueOnce(firstMore.promise)
    .mockReturnValueOnce(refresh.promise)
    .mockReturnValueOnce(secondMore.promise);
  act(() => {
    void result.current.loadMore();
  });
  act(() => {
    void result.current.refresh();
  });
  act(() => {
    void result.current.loadMore();
  });
  await waitFor(() => expect(getTimeline).toHaveBeenCalledTimes(4));
  await act(async () => {
    firstMore.resolve(page([noteItem('9', 'stale')], 'zz'));
  });
  expect(result.current.items.map((i) => i.id)).toEqual(['1']);
  expect(result.current.loadingMore).toBe(true);
});

test('a request started for the previous contact stays stale after the switch, even if a new one starts', async () => {
  const oldContact = deferred<TimelineResponse>();
  const newContact = deferred<TimelineResponse>();
  vi.mocked(getTimeline)
    .mockReturnValueOnce(oldContact.promise)
    .mockReturnValueOnce(newContact.promise);
  const { result, rerender } = renderHook(({ id }) => useTimeline(id), {
    initialProps: { id: 1 as number | undefined },
  });
  act(() => {
    void result.current.refresh();
  });
  rerender({ id: 2 });
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(getTimeline).toHaveBeenCalledTimes(2));
  await act(async () => {
    oldContact.resolve(page([noteItem('1', 'contact one')], 'x'));
  });
  expect(result.current.items).toEqual([]);
  expect(result.current.loading).toBe(true);
});
