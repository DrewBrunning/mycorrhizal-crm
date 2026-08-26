import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  confirmTwoFactor,
  disableTwoFactor,
  getTwoFactorStatus,
  regenerateRecoveryCodes,
  setupTwoFactor,
  updateSelfContact,
} from './users';

afterEach(() => {
  vi.unstubAllGlobals();
});

// T90: PATCH /users/me/self-contact wrapper. handleResponse (errorHandling.ts)
// reads response.text(), so the mocks provide both text and json.
function responseBody(body: unknown) {
  const text = JSON.stringify(body);
  return { ok: true, json: async () => body, text: async () => text };
}

describe('updateSelfContact', () => {
  test('PATCHes the chosen uid to /users/me/self-contact', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        responseBody({ message: 'Self contact updated', self_contact_vcard_uid: 'uid-1' }),
      );
    vi.stubGlobal('fetch', fetchMock);

    await updateSelfContact('uid-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/me/self-contact');
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body)).toEqual({ vcard_uid: 'uid-1' });
  });

  test('sends a null vcard_uid to clear the link', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(responseBody({ message: 'Self contact cleared' }));
    vi.stubGlobal('fetch', fetchMock);

    await updateSelfContact(null);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/me/self-contact');
    expect(JSON.parse(init.body)).toEqual({ vcard_uid: null });
  });

  test('throws the backend reason on a failed request', async () => {
    const body = {
      error: { code: 'NOT_FOUND', message: 'Contact not found', details: { reason: 'not yours' } },
    };
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: async () => body,
      text: async () => JSON.stringify(body),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(updateSelfContact('uid-x')).rejects.toThrow('not yours');
  });
});

// N8 (issue #158): 2FA management API wrappers.
describe('two-factor API', () => {
  test('getTwoFactorStatus GETs /users/2fa/status and maps enabled', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(responseBody({ enabled: true }));
    vi.stubGlobal('fetch', fetchMock);

    const status = await getTwoFactorStatus();
    expect(status).toEqual({ enabled: true });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/2fa/status');
    expect(init.method).toBe('GET');
  });

  test('setupTwoFactor POSTs /users/2fa/setup and returns secret + otpauth url', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        responseBody({ secret: 'JBSWY3DPEHPK3PXP', otpauth_url: 'otpauth://totp/...' }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const result = await setupTwoFactor();
    expect(result.secret).toBe('JBSWY3DPEHPK3PXP');
    expect(result.otpauth_url).toContain('otpauth://');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/2fa/setup');
    expect(init.method).toBe('POST');
  });

  test('confirmTwoFactor POSTs the code and returns the recovery codes', async () => {
    const codes = ['AAAAA-BBBBB-CCCCC', 'DDDDD-EEEEE-FFFFF'];
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(responseBody({ message: 'enabled', recovery_codes: codes }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await confirmTwoFactor('123456');
    expect(result.recovery_codes).toEqual(codes);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/2fa/confirm');
    expect(JSON.parse(init.body)).toEqual({ code: '123456' });
  });

  test('disableTwoFactor POSTs /users/2fa/disable with the code', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(responseBody({ message: 'disabled' }));
    vi.stubGlobal('fetch', fetchMock);

    await disableTwoFactor('123456');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/2fa/disable');
    expect(JSON.parse(init.body)).toEqual({ code: '123456' });
  });

  test('regenerateRecoveryCodes POSTs and returns the fresh codes', async () => {
    const codes = ['GGGGG-HHHHH-IIIII'];
    const fetchMock = vi.fn().mockResolvedValueOnce(responseBody({ recovery_codes: codes }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await regenerateRecoveryCodes('654321');
    expect(result.recovery_codes).toEqual(codes);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/2fa/recovery-codes/regenerate');
    expect(JSON.parse(init.body)).toEqual({ code: '654321' });
  });

  test('a failed 2FA call surfaces the backend message', async () => {
    const body = {
      error: {
        code: 'INVALID_INPUT',
        message: 'x',
        details: { reason: 'Invalid code. Please try again.' },
      },
    };
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: async () => body,
      text: async () => JSON.stringify(body),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(confirmTwoFactor('000000')).rejects.toThrow('Invalid code. Please try again.');
  });
});
