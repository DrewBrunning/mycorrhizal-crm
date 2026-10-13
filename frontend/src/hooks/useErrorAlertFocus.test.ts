import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { useErrorAlertFocus } from './useErrorAlertFocus';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

function attachRef(result: { current: ReturnType<typeof useErrorAlertFocus> }) {
  const el = document.createElement('div');
  el.tabIndex = -1;
  document.body.appendChild(el);
  const focus = vi.spyOn(el, 'focus');
  (result.current.errorRef as { current: HTMLDivElement | null }).current = el;
  return { el, focus };
}

test('starts with an empty error by default, or the given initial message', () => {
  expect(renderHook(() => useErrorAlertFocus()).result.current.error).toBe('');
  expect(renderHook(() => useErrorAlertFocus('boom')).result.current.error).toBe('boom');
  expect(renderHook(() => useErrorAlertFocus(() => 'lazy')).result.current.error).toBe('lazy');
});

test('a lazy initializer is invoked only once', () => {
  const init = vi.fn(() => 'lazy');
  const { rerender } = renderHook(() => useErrorAlertFocus(init));
  rerender();
  rerender();
  expect(init).toHaveBeenCalledTimes(1);
});

test('reporting an error focuses the alert element', () => {
  const { result } = renderHook(() => useErrorAlertFocus());
  const { focus } = attachRef(result);
  act(() => result.current.setError('Passwords do not match'));
  expect(result.current.error).toBe('Passwords do not match');
  expect(focus).toHaveBeenCalledTimes(1);
});

test('reporting the identical message again still moves focus', () => {
  const { result } = renderHook(() => useErrorAlertFocus());
  const { focus } = attachRef(result);
  act(() => result.current.setError('same'));
  act(() => result.current.setError('same'));
  expect(focus).toHaveBeenCalledTimes(2);
});

test('clearing the error does not steal focus', () => {
  const { result } = renderHook(() => useErrorAlertFocus());
  const { focus } = attachRef(result);
  act(() => result.current.setError('oops'));
  focus.mockClear();
  act(() => result.current.setError(''));
  expect(result.current.error).toBe('');
  expect(focus).not.toHaveBeenCalled();
});

test('an initial non-empty error focuses the alert once it is attached', () => {
  const { result, rerender } = renderHook(() => useErrorAlertFocus('preset'));
  // No ref yet at first effect; a later nonce change must still focus.
  const { focus } = attachRef(result);
  act(() => result.current.setError('next'));
  rerender();
  expect(focus).toHaveBeenCalledTimes(1);
});

test('setError keeps a stable identity across renders', () => {
  const { result, rerender } = renderHook(() => useErrorAlertFocus());
  const first = result.current.setError;
  act(() => result.current.setError('x'));
  rerender();
  expect(result.current.setError).toBe(first);
});

test('no focus happens before any error was reported', () => {
  const { result, rerender } = renderHook(() => useErrorAlertFocus());
  const { focus } = attachRef(result);
  rerender();
  expect(focus).not.toHaveBeenCalled();
});
