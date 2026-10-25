import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type SearchResult, searchAll } from '../api/search';
import { useSearch } from './useSearch';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/search', () => ({ searchAll: vi.fn() }));

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(searchAll).mockReset();
});

const hit = { notes: [{ ID: 1 }], activities: [] } as unknown as SearchResult;

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('starts with no result, no error, and not loading', () => {
  const { result } = renderHook(() => useSearch());
  expect(result.current.result).toBeNull();
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
});

test('searches with the trimmed query and exposes the result while toggling loading', async () => {
  const d = deferred<SearchResult>();
  vi.mocked(searchAll).mockReturnValue(d.promise);
  const { result } = renderHook(() => useSearch());
  act(() => {
    void result.current.runSearch('  ada  ');
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(searchAll).toHaveBeenCalledWith('ada');
  await act(async () => {
    d.resolve(hit);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.result).toBe(hit);
  expect(result.current.error).toBeNull();
});

test('a two-character query is searched; a shorter one (after trimming) is not', async () => {
  vi.mocked(searchAll).mockResolvedValue(hit);
  const { result } = renderHook(() => useSearch());
  await act(async () => {
    await result.current.runSearch('ab');
  });
  expect(searchAll).toHaveBeenCalledWith('ab');
  expect(result.current.result).toBe(hit);

  vi.mocked(searchAll).mockClear();
  await act(async () => {
    await result.current.runSearch(' a ');
  });
  expect(searchAll).not.toHaveBeenCalled();
  expect(result.current.result).toBeNull();
  expect(result.current.loading).toBe(false);
});

test('a too-short query also clears an earlier error', async () => {
  vi.mocked(searchAll).mockRejectedValueOnce(new Error('bad'));
  const { result } = renderHook(() => useSearch());
  await act(async () => {
    await result.current.runSearch('abc');
  });
  expect(result.current.error).toBe('bad');
  await act(async () => {
    await result.current.runSearch('');
  });
  expect(result.current.error).toBeNull();
});

test('a failed search sets the error, drops the previous result, logs, and stops loading', async () => {
  vi.mocked(searchAll).mockResolvedValueOnce(hit).mockRejectedValueOnce(new Error('timeout'));
  const { result } = renderHook(() => useSearch());
  await act(async () => {
    await result.current.runSearch('abc');
  });
  expect(result.current.result).toBe(hit);
  await act(async () => {
    await result.current.runSearch('xyz');
  });
  expect(result.current.error).toBe('timeout');
  expect(result.current.result).toBeNull();
  expect(result.current.loading).toBe(false);
  expect(console.error).toHaveBeenCalledWith('[searching] Error:', expect.anything());
});

test('a new search clears the previous error while it is in flight', async () => {
  vi.mocked(searchAll).mockRejectedValueOnce(new Error('bad'));
  const { result } = renderHook(() => useSearch());
  await act(async () => {
    await result.current.runSearch('abc');
  });
  const d = deferred<SearchResult>();
  vi.mocked(searchAll).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.runSearch('abc');
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve(hit);
  });
});

test('runSearch keeps a stable identity across renders', () => {
  const { result, rerender } = renderHook(() => useSearch());
  const first = result.current.runSearch;
  rerender();
  expect(result.current.runSearch).toBe(first);
});
