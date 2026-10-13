import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type DuplicatePair, dismissDuplicatePair, getDuplicatePairs } from '../api/duplicates';
import { useDuplicatePairs } from './useDuplicatePairs';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/duplicates', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/duplicates')>();
  return { ...actual, getDuplicatePairs: vi.fn(), dismissDuplicatePair: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getDuplicatePairs).mockReset();
  vi.mocked(dismissDuplicatePair).mockReset();
});

function pair(a: string, b: string): DuplicatePair {
  return { a: { uid: a }, b: { uid: b } } as unknown as DuplicatePair;
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('starts empty and idle', () => {
  const { result } = renderHook(() => useDuplicatePairs());
  expect(result.current.pairs).toEqual([]);
  expect(result.current.total).toBe(0);
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
});

test('a single page is fetched once with page 1 and a 100-row limit', async () => {
  vi.mocked(getDuplicatePairs).mockResolvedValue({ pairs: [pair('a', 'b')], total: 1 } as never);
  const { result } = renderHook(() => useDuplicatePairs());
  await act(async () => {
    await result.current.refresh();
  });
  expect(getDuplicatePairs).toHaveBeenCalledTimes(1);
  expect(getDuplicatePairs).toHaveBeenCalledWith({ page: 1, limit: 100 });
  expect(result.current.pairs).toHaveLength(1);
  expect(result.current.total).toBe(1);
  expect(result.current.loading).toBe(false);
});

test('pages are appended in order until the reported total is reached, filling incrementally', async () => {
  const d2 = deferred<never>();
  vi.mocked(getDuplicatePairs)
    .mockResolvedValueOnce({ pairs: [pair('a', 'b'), pair('c', 'd')], total: 3 } as never)
    .mockReturnValueOnce(d2.promise);
  const { result } = renderHook(() => useDuplicatePairs());
  act(() => {
    void result.current.refresh();
  });
  // The first page is visible while the second is still in flight.
  await waitFor(() => expect(result.current.pairs).toHaveLength(2));
  expect(result.current.loading).toBe(true);
  expect(result.current.total).toBe(0);
  expect(getDuplicatePairs).toHaveBeenLastCalledWith({ page: 2, limit: 100 });

  await act(async () => {
    d2.resolve({ pairs: [pair('e', 'f')], total: 3 } as never);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.pairs.map((p) => p.a.uid)).toEqual(['a', 'c', 'e']);
  expect(result.current.total).toBe(3);
  expect(getDuplicatePairs).toHaveBeenCalledTimes(2);
});

test('an empty scan result stops after one request', async () => {
  vi.mocked(getDuplicatePairs).mockResolvedValue({ pairs: [], total: 0 } as never);
  const { result } = renderHook(() => useDuplicatePairs());
  await act(async () => {
    await result.current.refresh();
  });
  expect(getDuplicatePairs).toHaveBeenCalledTimes(1);
  expect(result.current.total).toBe(0);
});

test('the page loop is capped at 50 requests even if the total is never reached', async () => {
  vi.mocked(getDuplicatePairs).mockResolvedValue({
    pairs: [pair('a', 'b')],
    total: 10_000,
  } as never);
  const { result } = renderHook(() => useDuplicatePairs());
  await act(async () => {
    await result.current.refresh();
  });
  expect(getDuplicatePairs).toHaveBeenCalledTimes(50);
  expect(result.current.pairs).toHaveLength(50);
  expect(result.current.total).toBe(50);
});

test('a refresh restarts from an empty list and clears the previous error', async () => {
  vi.mocked(getDuplicatePairs).mockRejectedValueOnce(new Error('scan failed'));
  const { result } = renderHook(() => useDuplicatePairs());
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('scan failed');
  expect(result.current.loading).toBe(false);
  expect(console.error).toHaveBeenCalledWith('[scanning for duplicates] Error:', expect.anything());

  const d = deferred<never>();
  vi.mocked(getDuplicatePairs).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve({ pairs: [], total: 0 } as never);
  });
});

test('refresh drops the earlier results before the new scan lands', async () => {
  vi.mocked(getDuplicatePairs).mockResolvedValueOnce({
    pairs: [pair('a', 'b')],
    total: 1,
  } as never);
  const { result } = renderHook(() => useDuplicatePairs());
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.pairs).toHaveLength(1);
  const d = deferred<never>();
  vi.mocked(getDuplicatePairs).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.pairs).toEqual([]));
  await act(async () => {
    d.resolve({ pairs: [], total: 0 } as never);
  });
});

test('dismiss sends both uids (blank when missing) and then rescans', async () => {
  vi.mocked(getDuplicatePairs).mockResolvedValue({ pairs: [], total: 0 } as never);
  vi.mocked(dismissDuplicatePair).mockResolvedValue(undefined as never);
  const { result } = renderHook(() => useDuplicatePairs());
  await act(async () => {
    await result.current.dismiss(pair('a', 'b'));
  });
  expect(dismissDuplicatePair).toHaveBeenCalledWith('a', 'b');
  expect(getDuplicatePairs).toHaveBeenCalledTimes(1);

  await act(async () => {
    await result.current.dismiss({ a: {}, b: {} } as unknown as DuplicatePair);
  });
  expect(dismissDuplicatePair).toHaveBeenLastCalledWith('', '');
});

test('a failed dismiss notifies with the latest notifier, logs, rethrows, and skips the rescan', async () => {
  vi.mocked(dismissDuplicatePair).mockRejectedValue(new Error('denied'));
  const first = vi.fn();
  const second = vi.fn();
  const { result, rerender } = renderHook(({ n }) => useDuplicatePairs(n), {
    initialProps: { n: { showError: first } },
  });
  rerender({ n: { showError: second } });
  await act(async () => {
    await expect(result.current.dismiss(pair('a', 'b'))).rejects.toThrow('denied');
  });
  expect(first).not.toHaveBeenCalled();
  expect(second).toHaveBeenCalledWith('denied');
  expect(console.error).toHaveBeenCalledWith(
    '[dismissing duplicate pair] Error:',
    expect.anything(),
  );
  expect(getDuplicatePairs).not.toHaveBeenCalled();
});

test('each page replaces the visible list with a new array so the UI re-renders as the scan fills', async () => {
  const d2 = deferred<never>();
  vi.mocked(getDuplicatePairs)
    .mockResolvedValueOnce({ pairs: [pair('a', 'b')], total: 2 } as never)
    .mockReturnValueOnce(d2.promise);
  const { result } = renderHook(() => useDuplicatePairs());
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.pairs).toHaveLength(1));
  const afterFirstPage = result.current.pairs;
  await act(async () => {
    d2.resolve({ pairs: [pair('c', 'd')], total: 2 } as never);
  });
  await waitFor(() => expect(result.current.pairs).toHaveLength(2));
  expect(afterFirstPage).toHaveLength(1);
  expect(result.current.pairs).not.toBe(afterFirstPage);
});
