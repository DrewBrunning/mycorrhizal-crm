import { test, expect, LOGGED_OUT, loginUser } from './fixtures';

// Issue #419 — the session JWT is an httpOnly cookie (see #392), which keeps
// it out of reach of XSS. #392's csrf_posture_test.go pins the cookie flags
// server-side, by parsing Set-Cookie directly. This spec pins the same
// invariants from the *browser's* point of view — the surface an XSS, or a
// script reading page state, would actually see — so a future change can't
// silently start persisting a token to storage or the URL.
//
// Regression test, not a fix: frontend/src/auth.ts already only ever caches
// non-sensitive user info (`user_id`/`username`/`is_admin`/
// `self_contact_vcard_uid`) in localStorage under the `user_info` key, never
// a token. `getToken()` returns the placeholder string `'cookie-auth'`, not
// a real credential.

// Auth flows must start from logged-out browser so the test observes the
// storage/cookie state produced by a real login, not a pre-seeded one.
test.use({ storageState: LOGGED_OUT });

// A JWT is three base64url segments separated by dots. Deliberately
// unanchored (no ^$) and applied to the *raw* localStorage string values
// below -- `user_info`'s raw value is a JSON blob, and a token hidden
// inside one of its fields must still be caught, not just a value that is
// a JWT and nothing else.
const JWT_SHAPE = /\bey[\w-]+\.[\w-]+\.[\w-]+\b/;

// The complete set of localStorage keys the app is known to write on/around
// login, confirmed by grepping frontend/src for each: `user_info` (auth.ts,
// non-sensitive cached user info -- the one under test below), `i18nextLng`
// (i18next's standard language-detector plugin), `themePreference`
// (AppThemeProvider.tsx), `dateFormat` (DateFormatProvider.tsx). None hold a
// credential. A key outside this set must be deliberately added here rather
// than slipping in unnoticed.
const KNOWN_BENIGN_LOCAL_STORAGE_KEYS = ['user_info', 'i18nextLng', 'themePreference', 'dateFormat'];

// IndexedDB databases the app is known to create: `workbox-expiration` is
// the production service worker's cache-expiration plugin (service-worker.ts,
// only registered in the frontend-prod build this spec runs against -- see
// CLAUDE.md's frontend-dev/frontend-prod note), storing cache-entry
// metadata, not credentials.
const KNOWN_BENIGN_INDEXEDDB_NAMES = ['workbox-expiration'];

test.describe('Credential storage (issue #419)', () => {
  test('no auth credential in localStorage/sessionStorage after login', async ({ page }) => {
    await loginUser(page);

    const storageDump = await page.evaluate(() => {
      const dump = (storage: Storage): Record<string, string> => {
        const out: Record<string, string> = {};
        for (let i = 0; i < storage.length; i++) {
          const key = storage.key(i);
          if (key !== null) out[key] = storage.getItem(key) ?? '';
        }
        return out;
      };
      return { local: dump(localStorage), session: dump(sessionStorage) };
    });

    // sessionStorage: the app never uses it for auth state at all.
    expect(Object.keys(storageDump.session), 'sessionStorage should hold nothing').toHaveLength(0);

    // localStorage: only the known-benign keys, so an *added* key must be
    // deliberately taught to this test rather than slipping in unnoticed.
    expect(Object.keys(storageDump.local).sort()).toEqual([...KNOWN_BENIGN_LOCAL_STORAGE_KEYS].sort());

    // `user_info` specifically: every key present must be one of the
    // known-safe cached-user-info fields (no token field ever added to it).
    // Not asserting the full set is present -- `user_id`/`username` are
    // currently dropped by a separate, unrelated bug (data.ID/data.Username
    // case mismatch vs. the backend's lowercase id/username -- flagged
    // separately, not this ticket's concern) -- only that nothing
    // unexpected, i.e. credential-shaped, shows up.
    const userInfo = JSON.parse(storageDump.local['user_info']);
    const allowedUserInfoKeys = ['is_admin', 'self_contact_vcard_uid', 'user_id', 'username'];
    for (const key of Object.keys(userInfo)) {
      expect(allowedUserInfoKeys, `unexpected key '${key}' in cached user_info`).toContain(key);
    }

    // Second, independent guard: no value anywhere in localStorage looks
    // like a JWT, regardless of which key it might end up under -- catches a
    // token hidden inside one of the already-known-benign keys, not just a
    // brand new one.
    for (const [key, value] of Object.entries(storageDump.local)) {
      expect(JWT_SHAPE.test(value), `localStorage['${key}'] must not look like a JWT`).toBe(false);
    }
  });

  test('no IndexedDB databases beyond the known service-worker cache', async ({ page }) => {
    await loginUser(page);

    const databases = await page.evaluate(() => indexedDB.databases());
    const names = databases.map((d) => d.name).sort();
    expect(names, 'only the known-benign IndexedDB databases may exist').toEqual(
      [...KNOWN_BENIGN_INDEXEDDB_NAMES].sort()
    );
  });

  test('no credential in the URL after login', async ({ page }) => {
    await loginUser(page);

    const url = new URL(page.url());
    expect(url.search, 'URL query string must be empty after login').toBe('');
    expect(url.hash, 'URL fragment must be empty after login').toBe('');
  });

  test('auth_token cookie is HttpOnly, SameSite=Strict, and not Secure on this HTTP stack', async ({
    page,
  }) => {
    await loginUser(page);

    const cookies = await page.context().cookies();
    const authCookie = cookies.find((c) => c.name === 'auth_token');
    expect(authCookie, 'auth_token cookie must be set after login').toBeTruthy();

    expect(authCookie!.httpOnly).toBe(true);
    expect(authCookie!.sameSite).toBe('Strict');
    // docker-compose.test.yml never sets COOKIE_SECURE, so it defaults to
    // false (see backend/config/config.go and securityHeaders.spec.ts's
    // matching HSTS-absent assertion) -- false here is the correct
    // assertion for "off", not a gap.
    expect(authCookie!.secure).toBe(false);
  });
});
