import { describe, expect, test, vi } from 'vitest';
import {
  notifySessionExpired,
  notifySessionRestored,
  onSessionExpired,
  onSessionRestored,
  onSessionSignInRequest,
  requestSessionSignIn,
} from './sessionExpiry';

describe('sessionExpiry pub/sub', () => {
  test('a notification reaches the current subscriber', () => {
    const listener = vi.fn();
    const unsubscribe = onSessionExpired(listener);

    notifySessionExpired('passive');

    expect(listener).toHaveBeenCalledWith({ mode: 'passive' });
    unsubscribe();
  });

  test('every subscriber receives the notification', () => {
    const first = vi.fn();
    const second = vi.fn();
    const offFirst = onSessionExpired(first);
    const offSecond = onSessionExpired(second);

    notifySessionExpired('blocking');

    expect(first).toHaveBeenCalledWith({ mode: 'blocking' });
    expect(second).toHaveBeenCalledWith({ mode: 'blocking' });
    offFirst();
    offSecond();
  });

  test('unsubscribe stops further notifications without affecting the others', () => {
    const first = vi.fn();
    const second = vi.fn();
    const offFirst = onSessionExpired(first);
    const offSecond = onSessionExpired(second);

    offFirst();
    notifySessionExpired('passive');

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledWith({ mode: 'passive' });
    offSecond();
  });

  test('notifying with no subscriber does not throw', () => {
    expect(() => notifySessionExpired('passive')).not.toThrow();
  });

  test('session-restored listeners fire until unsubscribed', () => {
    const listener = vi.fn();
    const unsubscribe = onSessionRestored(listener);

    notifySessionRestored();
    expect(listener).toHaveBeenCalledTimes(1);

    unsubscribe();
    notifySessionRestored();
    expect(listener).toHaveBeenCalledTimes(1);
  });

  test('sign-in requests reach the gate listener until unsubscribed', () => {
    const listener = vi.fn();
    const unsubscribe = onSessionSignInRequest(listener);

    requestSessionSignIn();
    expect(listener).toHaveBeenCalledTimes(1);

    unsubscribe();
    requestSessionSignIn();
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
