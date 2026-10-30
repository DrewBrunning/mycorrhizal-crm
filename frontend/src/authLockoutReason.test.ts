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

  // Issue #2107 (MAINT-02 deprecation of the flat body, switched in v2.0.0 by
  // #2108): the target lockout is the standard envelope with code
  // ACCOUNT_LOCKED and the retry values under error.details. The client must
  // read either shape, so v2.0.0 does not strand a deployed web build.
  describe('target envelope shape (v2.0.0)', () => {
    const envelope = (error: Record<string, unknown> = {}) => ({
      error: {
        code: 'ACCOUNT_LOCKED',
        message: fallback,
        reason_code: 'auth_account_locked',
        reason_params: { retry_after: 90 },
        details: { retry_after: 90, retry_after_at: '2026-01-01T00:00:00Z' },
        ...error,
      },
      request_id: 'req-1',
      timestamp: '2026-01-01T00:00:00Z',
    });

    test('login2FA localizes the lockout from the envelope reason', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValueOnce({ ok: false, status: 429, json: async () => envelope() }),
      );
      await expect(login2FA('000000')).rejects.toThrow(
        'Too many failed attempts. Try again in 90 s',
      );
    });

    test('login2FA takes retry_after from error.details when reason_params is absent', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () => envelope({ reason_params: undefined }),
        }),
      );
      await expect(login2FA('000000')).rejects.toThrow(
        'Too many failed attempts. Try again in 90 s',
      );
    });

    test('login2FA takes a top-level retry_after when the flat body has no reason_params', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () => lockedBody({ reason_params: undefined }),
        }),
      );
      await expect(login2FA('000000')).rejects.toThrow(
        'Too many failed attempts. Try again in 90 s',
      );
    });

    test('the retry_after fallback applies only to auth_account_locked', async () => {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () =>
            envelope({ reason_code: 'auth_not_a_real_code', reason_params: undefined }),
        }),
      );
      await expect(login2FA('000000')).rejects.toThrow(fallback);
    });

    test('passkey sign-in resolves the envelope lockout from details', async () => {
      stubWebAuthn();
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () => envelope({ reason_params: undefined }),
        }),
      );
      await expect(loginWithPasskey()).rejects.toThrow(
        'Too many failed attempts. Try again in 90 s',
      );
    });
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
          json: async () => lockedBody({ reason_params: {}, retry_after: undefined }),
        }),
    );
    await expect(login2FA('000000')).rejects.toThrow(fallback);
    await expect(login2FA('000000')).rejects.toThrow(fallback);
  });

  test('login2FA falls back when the 429 body is not an object', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce({ ok: false, status: 429, json: async () => null })
        .mockResolvedValueOnce({ ok: false, status: 429, json: async () => 'not json' }),
    );
    await expect(login2FA('000000')).rejects.toThrow(
      'Too many failed attempts. Please try again later.',
    );
    await expect(login2FA('000000')).rejects.toThrow(
      'Too many failed attempts. Please try again later.',
    );
  });

  test('login2FA falls back when the message is not a non-empty string', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () => lockedBody({ message: 42, reason_code: 'auth_not_a_real_code' }),
        })
        .mockResolvedValueOnce({
          ok: false,
          status: 429,
          json: async () => lockedBody({ message: '', reason_code: 'auth_not_a_real_code' }),
        }),
    );
    await expect(login2FA('000000')).rejects.toThrow(
      'Too many failed attempts. Please try again later.',
    );
    await expect(login2FA('000000')).rejects.toThrow(
      'Too many failed attempts. Please try again later.',
    );
  });

  test('passkey sign-in uses the envelope message when no reason resolves', async () => {
    stubWebAuthn();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({
        ok: false,
        status: 409,
        json: async () => ({ error: { code: 'CONFLICT', message: 'Server says no' } }),
      }),
    );
    await expect(loginWithPasskey()).rejects.toThrow('Server says no');
  });

  test('passkey sign-in resolves an envelope reason with params', async () => {
    stubWebAuthn();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({
        ok: false,
        status: 429,
        json: async () => ({
          error: {
            message: 'English fallback',
            reason_code: 'auth_account_locked',
            reason_params: { retry_after: 30 },
          },
        }),
      }),
    );
    await expect(loginWithPasskey()).rejects.toThrow('Too many failed attempts. Try again in 30 s');
  });

  test('passkey sign-in ignores a non-string reason and a non-object error', async () => {
    stubWebAuthn();
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce({
          ok: false,
          status: 409,
          json: async () => ({ error: { reason_code: 42, message: 'Server says no' } }),
        })
        .mockResolvedValueOnce({
          ok: false,
          status: 409,
          json: async () => ({ error: 'plain text' }),
        }),
    );
    await expect(loginWithPasskey()).rejects.toThrow('Server says no');
    await expect(loginWithPasskey()).rejects.toThrow('Unable to start passkey sign-in.');
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
