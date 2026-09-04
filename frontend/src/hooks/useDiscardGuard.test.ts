import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import { useDiscardGuard } from './useDiscardGuard';

afterEach(() => {
  // This codebase's vitest setup has no auto-cleanup: without unmounting,
  // an earlier test's still-live beforeunload listener would fire alongside
  // (or instead of) the current test's hook instance.
  cleanup();
  vi.restoreAllMocks();
});

describe('useDiscardGuard', () => {
  test('closes immediately when not dirty', () => {
    const { result } = renderHook(() => useDiscardGuard(false));
    const closeFn = vi.fn();

    act(() => result.current.guardedClose(closeFn));

    expect(closeFn).toHaveBeenCalledTimes(1);
    expect(result.current.confirmDialogProps.open).toBe(false);
  });

  test('opens the confirm dialog instead of closing when dirty', () => {
    const { result } = renderHook(() => useDiscardGuard(true));
    const closeFn = vi.fn();

    act(() => result.current.guardedClose(closeFn));

    expect(closeFn).not.toHaveBeenCalled();
    expect(result.current.confirmDialogProps.open).toBe(true);
  });

  test('onKeepEditing closes the confirm dialog without running the close function', () => {
    const { result } = renderHook(() => useDiscardGuard(true));
    const closeFn = vi.fn();
    act(() => result.current.guardedClose(closeFn));

    act(() => result.current.confirmDialogProps.onKeepEditing());

    expect(closeFn).not.toHaveBeenCalled();
    expect(result.current.confirmDialogProps.open).toBe(false);
  });

  test('onDiscard runs the pending close function and closes the confirm dialog', () => {
    const { result } = renderHook(() => useDiscardGuard(true));
    const closeFn = vi.fn();
    act(() => result.current.guardedClose(closeFn));

    act(() => result.current.confirmDialogProps.onDiscard());

    expect(closeFn).toHaveBeenCalledTimes(1);
    expect(result.current.confirmDialogProps.open).toBe(false);
  });

  test('warns on beforeunload while dirty', () => {
    const { rerender } = renderHook(({ isDirty }) => useDiscardGuard(isDirty), {
      initialProps: { isDirty: false },
    });

    const clean = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent;
    window.dispatchEvent(clean);
    expect(clean.defaultPrevented).toBe(false);

    rerender({ isDirty: true });

    const dirty = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent;
    window.dispatchEvent(dirty);
    expect(dirty.defaultPrevented).toBe(true);
  });
});
