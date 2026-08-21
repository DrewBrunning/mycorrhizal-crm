import { describe, test, expect, vi, afterEach } from 'vitest';
import { requestPasswordReset, confirmPasswordReset, changePassword } from './auth';

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
    const fetchMock = vi.fn().mockResolvedValueOnce(
      okResponse({ message: 'Reset email sent.' })
    );
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
    const fetchMock = vi.fn().mockResolvedValueOnce(
      okResponse({ message: 'Password reset complete.' })
    );
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
    const fetchMock = vi.fn().mockResolvedValueOnce(
      okResponse({ message: 'Password changed.' })
    );
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
