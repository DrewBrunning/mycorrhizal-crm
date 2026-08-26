import { afterEach, describe, expect, test, vi } from 'vitest';
import { fetchAndCacheUserInfo, getCachedSelfContactVCardUID, login2FA, loginUser } from './auth';

afterEach(() => {
  vi.unstubAllGlobals();
});

const USER_INFO_KEY = 'user_info';

describe('getCachedSelfContactVCardUID', () => {
  test('returns null when nothing is cached', () => {
    localStorage.removeItem(USER_INFO_KEY);
    expect(getCachedSelfContactVCardUID()).toBeNull();
  });

  test('returns the cached self-contact uid', () => {
    localStorage.setItem(
      USER_INFO_KEY,
      JSON.stringify({
        user_id: 1,
        username: 'u',
        is_admin: false,
        self_contact_vcard_uid: 'uid-1',
      }),
    );
    expect(getCachedSelfContactVCardUID()).toBe('uid-1');
  });

  test('treats a cache with no self contact as unset', () => {
    localStorage.setItem(
      USER_INFO_KEY,
      JSON.stringify({ user_id: 1, username: 'u', is_admin: false }),
    );
    expect(getCachedSelfContactVCardUID()).toBeNull();
  });

  test('returns null for a corrupt cache', () => {
    localStorage.setItem(USER_INFO_KEY, '{not json');
    expect(getCachedSelfContactVCardUID()).toBeNull();
  });
});

describe('fetchAndCacheUserInfo', () => {
  test('caches the self-contact uid returned by /users/me', async () => {
    // /users/me (models.CurrentUserResponse, backend/models/dtos.go) serializes
    // with lowercase JSON tags -- mock the real wire shape, not PascalCase.
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        id: 1,
        username: 'u',
        is_admin: false,
        self_contact_vcard_uid: 'uid-9',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const info = await fetchAndCacheUserInfo();

    expect(info?.self_contact_vcard_uid).toBe('uid-9');
    const cached = JSON.parse(localStorage.getItem(USER_INFO_KEY) || '{}');
    expect(cached.self_contact_vcard_uid).toBe('uid-9');
  });

  test('caches a null self contact when the server has none', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 1, username: 'u', is_admin: false }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const info = await fetchAndCacheUserInfo();

    expect(info?.self_contact_vcard_uid).toBeNull();
    const cached = JSON.parse(localStorage.getItem(USER_INFO_KEY) || '{}');
    expect(cached.self_contact_vcard_uid).toBeNull();
  });

  test('returns null (and leaves the cache alone) when the request fails', async () => {
    localStorage.setItem(USER_INFO_KEY, JSON.stringify({ self_contact_vcard_uid: 'uid-old' }));
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: false });
    vi.stubGlobal('fetch', fetchMock);

    const info = await fetchAndCacheUserInfo();
    expect(info).toBeNull();
    expect(JSON.parse(localStorage.getItem(USER_INFO_KEY) || '{}').self_contact_vcard_uid).toBe(
      'uid-old',
    );
  });

  // Regression test for the case mismatch where auth.ts read data.ID/data.Username
  // (PascalCase) against a lowercase `id`/`username` wire response: both fields
  // silently came back undefined and JSON.stringify dropped them from the cache.
  test('populates user_id and username from the lowercase /users/me response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 42, username: 'bob', is_admin: true, self_contact_vcard_uid: null }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const info = await fetchAndCacheUserInfo();

    expect(info?.user_id).toBe(42);
    expect(info?.username).toBe('bob');
    const cached = JSON.parse(localStorage.getItem(USER_INFO_KEY) || '{}');
    expect(cached.user_id).toBe(42);
    expect(cached.username).toBe('bob');
  });
});

// N8 (issue #158): two-step login for 2FA-enabled accounts.
describe('two-factor login', () => {
  test('loginUser reports two_factor_required without caching user info', async () => {
    localStorage.removeItem(USER_INFO_KEY);
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ two_factor_required: true }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await loginUser('alice', 'correct-password');

    expect(result.two_factor_required).toBe(true);
    expect(localStorage.getItem(USER_INFO_KEY)).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test('loginUser caches user info when 2FA is not required', async () => {
    localStorage.removeItem(USER_INFO_KEY);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ language: 'en', date_format: 'eu' }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ id: 1, username: 'alice', is_admin: false }),
      });
    vi.stubGlobal('fetch', fetchMock);

    const result = await loginUser('alice', 'correct-password');

    expect(result.two_factor_required).toBeUndefined();
    expect(result.language).toBe('en');
    expect(JSON.parse(localStorage.getItem(USER_INFO_KEY) || '{}').username).toBe('alice');
  });

  test('login2FA posts the code and caches user info on success', async () => {
    localStorage.removeItem(USER_INFO_KEY);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ language: 'de', date_format: 'eu' }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ id: 1, username: 'alice', is_admin: false }),
      });
    vi.stubGlobal('fetch', fetchMock);

    const result = await login2FA('123456');

    expect(result.language).toBe('de');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/login/2fa');
    expect(JSON.parse(init.body)).toEqual({ code: '123456' });
    expect(JSON.parse(localStorage.getItem(USER_INFO_KEY) || '{}').user_id).toBe(1);
  });

  test('login2FA throws when the code is rejected', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: false, status: 400 });
    vi.stubGlobal('fetch', fetchMock);

    await expect(login2FA('000000')).rejects.toThrow('Invalid code');
  });

  test('login2FA surfaces the backend lockout message on 429', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 429,
      json: async () => ({ message: 'Account temporarily locked. Try again in 60 seconds.' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(login2FA('000000')).rejects.toThrow(
      'Account temporarily locked. Try again in 60 seconds.',
    );
  });
});
