import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  AccountDeletionRequiresPromotionError,
  changePassword,
  confirmPasswordReset,
  deleteOwnAccount,
  requestPasswordReset,
} from './auth';

afterEach(() => {
  vi.unstubAllGlobals();
});

const okResponse = (body: unknown) => ({
  ok: true,
  status: 200,
  text: async () => JSON.stringify(body),
});

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  text: async () => JSON.stringify({ error: { message: 'Invalid token' } }),
});

describe('requestPasswordReset', () => {
  test('POSTs the email and returns the server message', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ message: 'Reset email sent.' }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await requestPasswordReset('a@example.com');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/password-reset/request');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ email: 'a@example.com' });
    expect(result).toBe('Reset email sent.');
  });

  test('returns the fallback message when the response has no message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));
    const result = await requestPasswordReset('a@example.com');
    expect(result).toBe('If an account exists, password reset instructions were sent.');
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(requestPasswordReset('a@example.com')).rejects.toThrow('Invalid token');
  });
});

describe('confirmPasswordReset', () => {
  test('POSTs the token and password and returns the server message', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(okResponse({ message: 'Password reset complete.' }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await confirmPasswordReset('tok-1', 'newpass');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/password-reset/confirm');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ token: 'tok-1', password: 'newpass' });
    expect(result).toBe('Password reset complete.');
  });

  test('returns the fallback message when the response has no message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));
    const result = await confirmPasswordReset('tok-1', 'newpass');
    expect(result).toBe('Password reset successful.');
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(confirmPasswordReset('tok-1', 'newpass')).rejects.toThrow('Invalid token');
  });
});

describe('changePassword', () => {
  test('POSTs the passwords and returns the server message', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ message: 'Password changed.' }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await changePassword('oldpass', 'newpass');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/change-password');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ current_password: 'oldpass', new_password: 'newpass' });
    expect(result).toBe('Password changed.');
  });

  test('returns the fallback message when the response has no message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));
    const result = await changePassword('oldpass', 'newpass');
    expect(result).toBe('Password updated successfully.');
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(changePassword('oldpass', 'wrong')).rejects.toThrow('Invalid token');
  });
});

describe('deleteOwnAccount', () => {
  test('DELETEs with the password and returns the server message', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        okResponse({ message: 'Your account and all its data have been deleted.' }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const result = await deleteOwnAccount('correct-password');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/account');
    expect(init.method).toBe('DELETE');
    expect(JSON.parse(init.body)).toEqual({ current_password: 'correct-password' });
    expect(result).toBe('Your account and all its data have been deleted.');
  });

  test('includes totp_code and promote_user_id only when given', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ message: 'done' }));
    vi.stubGlobal('fetch', fetchMock);

    await deleteOwnAccount('correct-password', '123456', 7);

    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(init.body)).toEqual({
      current_password: 'correct-password',
      totp_code: '123456',
      promote_user_id: 7,
    });
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteOwnAccount('wrong-password')).rejects.toThrow('Invalid token');
  });

  test('throws AccountDeletionRequiresPromotionError with the candidate list on 409', async () => {
    const response = {
      ok: false,
      status: 409,
      statusText: 'Conflict',
      text: async () =>
        JSON.stringify({
          error: {
            code: 'CONFLICT',
            message: 'You are the only admin; choose another user to promote to admin first',
            details: {
              candidates: [
                { id: 2, username: 'alice' },
                { id: 3, username: 'bob' },
              ],
            },
          },
        }),
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(response));

    await expect(deleteOwnAccount('correct-password')).rejects.toSatisfy((err: unknown) => {
      expect(err).toBeInstanceOf(AccountDeletionRequiresPromotionError);
      const promotionErr = err as AccountDeletionRequiresPromotionError;
      expect(promotionErr.candidates).toEqual([
        { id: 2, username: 'alice' },
        { id: 3, username: 'bob' },
      ]);
      return true;
    });
  });

  test('a 409 with no candidates falls back to a plain error', async () => {
    const response = {
      ok: false,
      status: 409,
      statusText: 'Conflict',
      text: async () =>
        JSON.stringify({ error: { code: 'CONFLICT', message: 'some other conflict' } }),
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(response));

    await expect(deleteOwnAccount('correct-password')).rejects.toThrow('some other conflict');
  });
});
