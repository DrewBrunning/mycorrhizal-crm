import { describe, expect, test } from 'vitest';
import { isAppShellNavigation } from './serviceWorkerNavigation';

const navigation = (pathname: string) =>
  isAppShellNavigation({ request: { mode: 'navigate' }, url: { pathname } });

describe('isAppShellNavigation', () => {
  test('serves the app shell for SPA routes', () => {
    expect(navigation('/')).toBe(true);
    expect(navigation('/contacts')).toBe(true);
    expect(navigation('/contacts/abc-123/prep')).toBe(true);
    expect(navigation('/login')).toBe(true);
  });

  test('never serves the app shell for the OIDC login or callback navigation', () => {
    // Issue #2636: these are top-level navigations, not fetches. Answering
    // them with index.html reloads the SPA and the request never reaches the
    // backend, so SSO can never start or finish.
    expect(navigation('/api/v1/auth/oidc/login')).toBe(false);
    expect(navigation('/api/v1/auth/oidc/callback')).toBe(false);
  });

  test('never serves the app shell for any backend path', () => {
    for (const pathname of [
      '/api',
      '/api/v1/users/me',
      '/health',
      '/health/live',
      '/health/ready',
      '/mcp',
      '/mcp/',
      '/carddav/',
      '/carddav/contacts/user.vcf',
      '/caldav/',
      '/caldav/events',
      '/.well-known/carddav',
      '/.well-known/assetlinks.json',
    ]) {
      expect(navigation(pathname), pathname).toBe(false);
    }
  });

  test('does not treat SPA paths that merely start with a backend word as backend', () => {
    expect(navigation('/apiary')).toBe(true);
    expect(navigation('/healthcheck')).toBe(true);
    expect(navigation('/mcp-dashboard')).toBe(true);
  });

  test('skips recovery and file-extension navigations', () => {
    expect(navigation('/_recovery.html')).toBe(false);
    expect(navigation('/assets/index-abc123.js')).toBe(false);
  });

  test('skips anything that is not a navigation', () => {
    expect(
      isAppShellNavigation({
        request: { mode: 'cors' },
        url: { pathname: '/contacts' },
      }),
    ).toBe(false);
  });
});
