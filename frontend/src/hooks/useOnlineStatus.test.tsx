import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { useOnlineStatus } from './useOnlineStatus';

const originalOnLine = Object.getOwnPropertyDescriptor(window.navigator, 'onLine');

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, value });
}

afterEach(() => {
  cleanup();
  if (originalOnLine) {
    Object.defineProperty(window.navigator, 'onLine', originalOnLine);
  } else {
    delete (window.navigator as unknown as { onLine?: boolean }).onLine;
  }
});

test('starts from the browser status, so an offline reload paints offline immediately', () => {
  setOnline(false);
  const { result } = renderHook(() => useOnlineStatus());
  expect(result.current).toBe(false);
});

test('follows the online and offline events', () => {
  setOnline(true);
  const { result } = renderHook(() => useOnlineStatus());
  expect(result.current).toBe(true);

  act(() => {
    setOnline(false);
    window.dispatchEvent(new Event('offline'));
  });
  expect(result.current).toBe(false);

  act(() => {
    setOnline(true);
    window.dispatchEvent(new Event('online'));
  });
  expect(result.current).toBe(true);
});
