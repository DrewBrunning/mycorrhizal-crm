import { afterEach, describe, expect, test, vi } from 'vitest';
import { login2FA, loginWithPasskey } from './auth';
import { stubWebAuthn, unstubWebAuthn } from './webauthnTestUtils';

afterEach(() => {
  vi.unstubAllGlobals();
  unstubWebAuthn();
});

// Issue #2101 (ADR 0038): the account-lockout 429 is a flat body, not the
// error envelope, so its top-level reason_code/reason_params must resolve the
// same way the envelope's do.
describe('lockout 429 reason codes', () => {
  const fallback = 'Too many failed login attempts. Please try again later.';
  const lockedBody = (extra: Record<string, unknown> = {}) => ({
    error: 'Account temporarily locked',
    message: fallback,
    retry_after: 90,
    retry_after_at: '2026-01-01T00:00:00Z',
    reason_code: 'auth_account_locked',
    reason_params: { retry_after: 90 },
    ...extra,
  });

  test('login2FA shows the localized lockout text from the top-level reason', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({ ok: false, status: 429, json: async () => lockedBody() }),
    );
    await expect(login2FA('000000')).rejects.toThrow('Too many failed attempts. Try again in 90 s');
  });

  test('login2FA falls back to message for an unknown code or a missing param', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () => lockedBody({ reason_code: 'auth_not_a_real_code' }),
        })
        .mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () => lockedBody({ reason_params: {} }),
        }),
    );
    await expect(login2FA('000000')).rejects.toThrow(fallback);
    await expect(login2FA('000000')).rejects.toThrow(fallback);
  });

  test('passkey sign-in resolves the lockout reason and an envelope reason', async () => {
    stubWebAuthn();
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce({ ok: false, status: 429, json: async () => lockedBody() })
        .mockResolvedValueOnce({
          ok: false,
          status: 409,
          json: async () => ({
            error: {
              code: 'CONFLICT',
              message: 'English fallback',
              reason_code: 'auth_passkey_none_registered',
            },
          }),
        }),
    );
    await expect(loginWithPasskey()).rejects.toThrow('Too many failed attempts. Try again in 90 s');
    await expect(loginWithPasskey()).rejects.toThrow('No passkey is registered for this account');
  });
});
