import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { getGraph } from '../api/graph';
import type { GraphResponse } from '../types/graph';
import { useGraph } from './useGraph';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

vi.mock('../api/graph', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/graph')>();
  return { ...actual, getGraph: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getGraph).mockReset();
});

const graphResponse: GraphResponse = {
  nodes: [
    { id: 'c-1', type: 'contact', label: 'Alice' },
    { id: 'c-2', type: 'contact', label: 'Bob' },
  ],
  edges: [{ id: 'e-1', source: 'c-1', target: 'c-2', type: 'relationship', label: 'friend_of' }],
};

test('loads graph data on mount', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getGraph).mockResolvedValue(graphResponse);

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getGraph).toHaveBeenCalledTimes(1);
  expect(result.current.data).toEqual(graphResponse);
  expect(result.current.error).toBeNull();
});

test('refetch re-runs the request', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getGraph).mockResolvedValue(graphResponse);

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refetch();
  });

  expect(getGraph).toHaveBeenCalledTimes(2);
});

test('sets an auth error when not authenticated', async () => {
  localStorage.clear();

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('No authentication token found');
  expect(result.current.data).toBeNull();
  expect(getGraph).not.toHaveBeenCalled();
});

test('sets error when the fetch fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getGraph).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.data).toBeNull();
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

test('ignores a stale success that settles after a newer fetch', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  const first = deferred<GraphResponse>();
  const second = deferred<GraphResponse>();
  vi.mocked(getGraph).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

  const { result } = renderHook(() => useGraph());
  await act(async () => {});

  // Fire a refetch while the initial request is still in flight.
  const refetchPromise = result.current.refetch();
  await act(async () => {
    second.resolve(graphResponse);
    await refetchPromise;
  });
  expect(result.current.data).toEqual(graphResponse);

  // The stale first request settling afterwards must not clobber it.
  await act(async () => {
    first.resolve({ nodes: [], edges: [] });
  });
  expect(result.current.data).toEqual(graphResponse);
});

test('ignores a stale failure that settles after a newer fetch', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  const first = deferred<GraphResponse>();
  const second = deferred<GraphResponse>();
  vi.mocked(getGraph).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

  const { result } = renderHook(() => useGraph());
  await act(async () => {});

  const refetchPromise = result.current.refetch();
  await act(async () => {
    second.resolve(graphResponse);
    await refetchPromise;
  });
  expect(result.current.error).toBeNull();

  await act(async () => {
    first.reject(new Error('stale failure'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.data).toEqual(graphResponse);
});

test('defaults to empty arrays when the response omits nodes or edges', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getGraph).mockResolvedValue({} as GraphResponse);

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.data).toEqual({ nodes: [], edges: [] });
});
