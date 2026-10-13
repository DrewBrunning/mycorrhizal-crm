import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { useDebouncedValue } from './useDebounce';

beforeEach(() => {
  vi.useFakeTimers();
});

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

test('returns the initial value immediately', async () => {
  const { result } = renderHook(() => useDebouncedValue('a'));
  expect(result.current).toBe('a');
});

test('adopts a new value only after the default 350ms delay', async () => {
  const { result, rerender } = renderHook(({ v }) => useDebouncedValue(v), {
    initialProps: { v: 'a' },
  });
  rerender({ v: 'b' });
  await act(async () => {
    vi.advanceTimersByTime(349);
  });
  expect(result.current).toBe('a');
  await act(async () => {
    vi.advanceTimersByTime(1);
  });
  expect(result.current).toBe('b');
});

test('honors a custom delay', async () => {
  const { result, rerender } = renderHook(({ v }) => useDebouncedValue(v, 50), {
    initialProps: { v: 1 },
  });
  rerender({ v: 2 });
  await act(async () => {
    vi.advanceTimersByTime(49);
  });
  expect(result.current).toBe(1);
  await act(async () => {
    vi.advanceTimersByTime(1);
  });
  expect(result.current).toBe(2);
});

test('rapid changes restart the timer and only the last value lands', async () => {
  const { result, rerender } = renderHook(({ v }) => useDebouncedValue(v, 100), {
    initialProps: { v: 'a' },
  });
  rerender({ v: 'b' });
  await act(async () => {
    vi.advanceTimersByTime(60);
  });
  rerender({ v: 'c' });
  await act(async () => {
    vi.advanceTimersByTime(60);
  });
  expect(result.current).toBe('a');
  await act(async () => {
    vi.advanceTimersByTime(40);
  });
  expect(result.current).toBe('c');
});

test('a changed delay restarts the pending timer', async () => {
  const { result, rerender } = renderHook(({ d }) => useDebouncedValue('x', d), {
    initialProps: { d: 100 },
  });
  rerender({ d: 500 });
  await act(async () => {
    vi.advanceTimersByTime(200);
  });
  expect(result.current).toBe('x');
});

test('unmounting cancels the pending update', async () => {
  const clear = vi.spyOn(window, 'clearTimeout');
  const { rerender, unmount } = renderHook(({ v }) => useDebouncedValue(v, 100), {
    initialProps: { v: 'a' },
  });
  rerender({ v: 'b' });
  unmount();
  expect(clear).toHaveBeenCalled();
});
