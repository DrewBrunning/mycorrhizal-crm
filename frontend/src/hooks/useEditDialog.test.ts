import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { useEditDialog } from './useEditDialog';

afterEach(cleanup);

test('starts closed with nothing being edited', () => {
  const { result } = renderHook(() => useEditDialog<{ id: string }>());
  expect(result.current.open).toBe(false);
  expect(result.current.editing).toBeNull();
});

test('openEdit opens on the item; openCreate reopens with no item', () => {
  const { result } = renderHook(() => useEditDialog<{ id: string }>());
  act(() => result.current.openEdit({ id: 'a' }));
  expect(result.current.open).toBe(true);
  expect(result.current.editing).toEqual({ id: 'a' });

  act(() => result.current.openCreate());
  expect(result.current.open).toBe(true);
  expect(result.current.editing).toBeNull();
});

test('close clears the edited item too', () => {
  const { result } = renderHook(() => useEditDialog<{ id: string }>());
  act(() => result.current.openEdit({ id: 'a' }));
  act(() => result.current.close());
  expect(result.current.open).toBe(false);
  expect(result.current.editing).toBeNull();
});

test('the callbacks are stable across renders', () => {
  const { result, rerender } = renderHook(() => useEditDialog<string>());
  const first = result.current;
  act(() => result.current.openEdit('x'));
  rerender();
  expect(result.current.openCreate).toBe(first.openCreate);
  expect(result.current.openEdit).toBe(first.openEdit);
  expect(result.current.close).toBe(first.close);
});
