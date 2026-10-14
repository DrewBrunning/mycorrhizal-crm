import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { useRowKeys } from './useRowKeys';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

const keysOf = (r: ReturnType<typeof useRowKeys>, n: number) =>
  Array.from({ length: n }, (_, i) => r.keyAt(i));

test('hands out distinct keys, one per row, counting up from zero', () => {
  const { result } = renderHook(() => useRowKeys(3));
  expect(keysOf(result.current, 3)).toEqual([0, 1, 2]);
});

test('removing a middle row keeps the keys of the rows that stay', () => {
  const { result, rerender } = renderHook(({ n }) => useRowKeys(n), { initialProps: { n: 3 } });
  act(() => result.current.onRemove(1));
  rerender({ n: 2 });
  expect(keysOf(result.current, 2)).toEqual([0, 2]);
});

test('adding a row appends a fresh key that never reuses a removed one', () => {
  const { result, rerender } = renderHook(({ n }) => useRowKeys(n), { initialProps: { n: 3 } });
  act(() => result.current.onRemove(2));
  rerender({ n: 2 });
  act(() => result.current.onAdd());
  rerender({ n: 3 });
  expect(keysOf(result.current, 3)).toEqual([0, 1, 3]);
});

test('a longer list pads keys from the end without disturbing existing ones', () => {
  const { result, rerender } = renderHook(({ n }) => useRowKeys(n), { initialProps: { n: 2 } });
  rerender({ n: 4 });
  expect(keysOf(result.current, 4)).toEqual([0, 1, 2, 3]);
});

test('a shorter list truncates keys from the end', () => {
  const { result, rerender } = renderHook(({ n }) => useRowKeys(n), { initialProps: { n: 4 } });
  rerender({ n: 2 });
  expect(keysOf(result.current, 2)).toEqual([0, 1]);
  expect(result.current.keyAt(2)).toBeUndefined();
});

test('an empty list has no keys, and onAdd then external growth stays consistent', () => {
  const { result, rerender } = renderHook(({ n }) => useRowKeys(n), { initialProps: { n: 0 } });
  expect(result.current.keyAt(0)).toBeUndefined();
  act(() => result.current.onAdd());
  rerender({ n: 1 });
  expect(keysOf(result.current, 1)).toEqual([0]);
});

test('repeated onAdd calls keep every key unique, including keys padded afterwards', () => {
  const { result, rerender } = renderHook(({ n }) => useRowKeys(n), { initialProps: { n: 2 } });
  act(() => result.current.onAdd());
  act(() => result.current.onAdd());
  act(() => result.current.onAdd());
  rerender({ n: 6 });
  expect(keysOf(result.current, 6)).toEqual([0, 1, 2, 3, 4, 5]);
});

test('shrinking without onRemove discards the trailing keys, so regrowing hands out fresh ones', () => {
  const { result, rerender } = renderHook(({ n }) => useRowKeys(n), { initialProps: { n: 3 } });
  rerender({ n: 1 });
  rerender({ n: 3 });
  expect(keysOf(result.current, 3)).toEqual([0, 3, 4]);
});
