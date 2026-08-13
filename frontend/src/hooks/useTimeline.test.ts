import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, act, waitFor } from '@testing-library/react';
import { useTimeline } from './useTimeline';
import { getTimeline, TimelineItem, TimelineResponse } from '../api/timeline';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('../api/timeline', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/timeline')>();
  return { ...actual, getTimeline: vi.fn() };
});

beforeEach(() => {
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
      () => new Promise((res) => { resolveLoadMore = res; })
    )
    // The refresh after the filter change hangs until we resolve it.
    .mockImplementationOnce(
      () => new Promise((res) => { resolveRefresh = res; })
    );

  const { result } = renderHook(() => useTimeline(42));

  await act(async () => { await result.current.refresh(); });
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

  await act(async () => { await result.current.refresh(); });
  expect(result.current.items.map((i) => i.id)).toEqual(['1']);

  // Navigate to contact 2: the page should clear before any refetch, so
  // contact A's rows never render under contact B.
  await act(async () => { rerender({ id: 2 }); });

  expect(result.current.items).toEqual([]);
  expect(result.current.nextCursor).toBe('');

  await act(async () => { await result.current.refresh(); });
  await waitFor(() => expect(result.current.items.map((i) => i.id)).toEqual(['2']));
});
