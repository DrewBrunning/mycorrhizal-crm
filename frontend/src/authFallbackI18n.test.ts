import { afterEach, describe, expect, test, vi } from 'vitest';
import { login2FA, loginUser, loginWithPasskey, loginWithPasskeyPasswordless } from './auth';
import i18n from './i18n/config';
import { requestOptionsWire, stubWebAuthn, unstubWebAuthn } from './webauthnTestUtils';

// #2920 review: when the server sends no usable message/reason code, auth.ts
// used to throw hardcoded English ("Login failed", "Invalid code", ...) in every
// UI language. The fallbacks now come from the active locale.

afterEach(async () => {
  vi.unstubAllGlobals();
  unstubWebAuthn();
  await i18n.changeLanguage('en');
});

const failWith = (status: number, body: unknown = {}) =>
  vi.fn().mockResolvedValue({ ok: false, status, json: async () => body });

const LANGS = ['de', 'es', 'fr', 'it'] as const;

describe.each(LANGS)('sign-in fallbacks in %s', (lang) => {
  const english = (key: string) => i18n.getFixedT('en')(key);
  const local = (key: string) => i18n.getFixedT(lang)(key);

  const expectLocalized = async (promise: Promise<unknown>, key: string) => {
    expect(local(key)).not.toBe(english(key));
    await expect(promise).rejects.toThrow(local(key));
  };

  test('password sign-in with an empty error body', async () => {
    await i18n.changeLanguage(lang);
    vi.stubGlobal('fetch', failWith(401));
    await expectLocalized(loginUser('a', 'b'), 'login.loginFailed');
  });

  test('2FA code rejected with an empty error body', async () => {
    await i18n.changeLanguage(lang);
    vi.stubGlobal('fetch', failWith(401));
    await expectLocalized(login2FA('000000'), 'login.invalidCode');
  });

  test('2FA 429 with an empty error body', async () => {
    await i18n.changeLanguage(lang);
    vi.stubGlobal('fetch', failWith(429));
    await expectLocalized(login2FA('000000'), 'login.tooManyAttempts');
  });

  test('passkey begin and finish failures', async () => {
    await i18n.changeLanguage(lang);
    stubWebAuthn();
    vi.stubGlobal('fetch', failWith(500));
    await expectLocalized(loginWithPasskey(), 'login.passkeyStartFailed');
    await expectLocalized(loginWithPasskeyPasswordless(), 'login.passkeyStartFailed');
  });

  test('passkey finish failure', async () => {
    await i18n.changeLanguage(lang);
    stubWebAuthn();
    const begin = {
      ok: true,
      status: 200,
      json: async () => requestOptionsWire,
    };
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(begin)
        .mockResolvedValue({ ok: false, status: 401, json: async () => ({}) }),
    );
    await expectLocalized(loginWithPasskey(), 'login.passkeyVerifyFailed');
  });
});

test('English fallbacks are unchanged', async () => {
  vi.stubGlobal('fetch', failWith(401));
  await expect(loginUser('a', 'b')).rejects.toThrow('Login failed');
  vi.stubGlobal('fetch', failWith(429));
  await expect(login2FA('1')).rejects.toThrow('Too many failed attempts. Please try again later.');
});
