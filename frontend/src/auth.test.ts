import { describe, test, expect, vi, afterEach } from 'vitest';
import { getCachedSelfContactVCardUID, fetchAndCacheUserInfo } from './auth';

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
      JSON.stringify({ user_id: 1, username: 'u', is_admin: false, self_contact_vcard_uid: 'uid-1' })
    );
    expect(getCachedSelfContactVCardUID()).toBe('uid-1');
  });

  test('treats a cache with no self contact as unset', () => {
    localStorage.setItem(USER_INFO_KEY, JSON.stringify({ user_id: 1, username: 'u', is_admin: false }));
    expect(getCachedSelfContactVCardUID()).toBeNull();
  });

  test('returns null for a corrupt cache', () => {
    localStorage.setItem(USER_INFO_KEY, '{not json');
    expect(getCachedSelfContactVCardUID()).toBeNull();
  });
});

describe('fetchAndCacheUserInfo', () => {
  test('caches the self-contact uid returned by /users/me', async () => {
    // auth.ts reads data.ID / data.Username (PascalCase) alongside the
    // lowercase response fields — mock what the module actually reads.
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ID: 1, Username: 'u', is_admin: false, self_contact_vcard_uid: 'uid-9' }),
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
      json: async () => ({ ID: 1, Username: 'u', is_admin: false }),
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
    expect(JSON.parse(localStorage.getItem(USER_INFO_KEY) || '{}').self_contact_vcard_uid).toBe('uid-old');
  });
});
