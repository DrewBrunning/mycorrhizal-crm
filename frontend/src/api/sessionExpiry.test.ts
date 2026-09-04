import { describe, expect, test, vi } from 'vitest';
import { notifySessionExpired, onSessionExpired } from './sessionExpiry';

describe('sessionExpiry pub/sub', () => {
  test('a notification reaches the current subscriber', () => {
    const listener = vi.fn();
    const unsubscribe = onSessionExpired(listener);

    notifySessionExpired('passive');

    expect(listener).toHaveBeenCalledWith({ mode: 'passive' });
    unsubscribe();
  });

  test('a second subscribe call replaces the first subscriber', () => {
    const first = vi.fn();
    const second = vi.fn();
    onSessionExpired(first);
    const unsubscribeSecond = onSessionExpired(second);

    notifySessionExpired('blocking');

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledWith({ mode: 'blocking' });
    unsubscribeSecond();
  });

  test('unsubscribe stops further notifications', () => {
    const listener = vi.fn();
    const unsubscribe = onSessionExpired(listener);
    unsubscribe();

    notifySessionExpired('passive');

    expect(listener).not.toHaveBeenCalled();
  });

  test('unsubscribing after being replaced is a no-op (does not clear the new subscriber)', () => {
    const first = vi.fn();
    const second = vi.fn();
    const unsubscribeFirst = onSessionExpired(first);
    onSessionExpired(second);

    unsubscribeFirst();
    notifySessionExpired('blocking');

    expect(second).toHaveBeenCalledWith({ mode: 'blocking' });
  });

  test('notifying with no subscriber does not throw', () => {
    expect(() => notifySessionExpired('passive')).not.toThrow();
  });
});
