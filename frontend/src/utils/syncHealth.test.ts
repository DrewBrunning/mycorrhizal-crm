import { describe, expect, test } from 'vitest';
import { daysSince, terminalReasonKey } from './syncHealth';

describe('terminalReasonKey', () => {
  test('maps each known backend failure-mode slug to its message key', () => {
    expect(terminalReasonKey('auth-expiry')).toBe('settings.syncHealth.terminalAuthExpiry');
    expect(terminalReasonKey('authz-revoked')).toBe('settings.syncHealth.terminalAuthzRevoked');
    expect(terminalReasonKey('remote-resource-deleted')).toBe(
      'settings.syncHealth.terminalRemoteDeleted',
    );
    expect(terminalReasonKey('client-error')).toBe('settings.syncHealth.terminalClientError');
  });

  test('falls back to the generic message for an unknown slug', () => {
    expect(terminalReasonKey('')).toBe('settings.syncHealth.terminalGeneric');
    expect(terminalReasonKey('something-new')).toBe('settings.syncHealth.terminalGeneric');
  });
});

describe('daysSince', () => {
  const now = Date.UTC(2026, 8, 3, 12, 0, 0);

  test('returns null for a null or unparseable timestamp', () => {
    expect(daysSince(null, now)).toBeNull();
    expect(daysSince('not-a-date', now)).toBeNull();
  });

  test('returns whole days elapsed, floored, never negative', () => {
    expect(daysSince(new Date(now).toISOString(), now)).toBe(0);
    expect(daysSince(new Date(now - 47 * 86_400_000).toISOString(), now)).toBe(47);
    expect(daysSince(new Date(now - 90 * 60_000).toISOString(), now)).toBe(0); // 90 min ago
    expect(daysSince(new Date(now + 5 * 86_400_000).toISOString(), now)).toBe(0); // future clamps
  });
});
