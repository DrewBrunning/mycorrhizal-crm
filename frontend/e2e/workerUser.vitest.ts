// Unit tests for the per-worker e2e user harness (issue #1480): the pure
// pieces (naming, the settings-mutation guard's matcher) and a structural gate
// that every Playwright spec takes its `test` from ./fixtures -- the only place
// the per-worker `storageState` override lives. Run by vitest (e2e/*.vitest.ts).
import * as fs from 'node:fs';
import * as path from 'node:path';
import { describe, expect, it } from 'vitest';
import { isSettingsMutation, RUN_ID, workerUserCredentials } from './workerUser';

const E2E_DIR = __dirname;

describe('workerUserCredentials', () => {
  it('names the account for the worker and run, with underscores only', () => {
    const c = workerUserCredentials(3, 'abc123');
    expect(c.username).toBe('e2e_w3_abc123');
    expect(c.email).toBe('e2e_w3_abc123@example.com');
    expect(c.password.length).toBeGreaterThanOrEqual(12);
    expect(c.username).toMatch(/^[a-z0-9_]+$/);
  });

  it('gives distinct accounts to distinct workers and to distinct runs', () => {
    expect(workerUserCredentials(0, 'r1').username).not.toBe(
      workerUserCredentials(1, 'r1').username,
    );
    expect(workerUserCredentials(0, 'r1').username).not.toBe(
      workerUserCredentials(0, 'r2').username,
    );
  });

  it('defaults to this process run id', () => {
    expect(RUN_ID).toMatch(/^[0-9a-f]{8}$/);
    expect(workerUserCredentials(0).username).toBe(`e2e_w0_${RUN_ID}`);
  });
});

describe('isSettingsMutation', () => {
  const base = 'http://localhost:7300/api/v1';

  it.each([
    ['PATCH', `${base}/users/enabled-contact-fields`],
    ['PATCH', `${base}/users/date-format`],
    ['PATCH', `${base}/users/language`],
    ['POST', `${base}/users/change-password`],
    ['PATCH', `${base}/users/me/self-contact`],
    ['POST', `${base}/users/2fa/setup`],
    ['PUT', `${base}/notifications/config`],
    ['POST', `${base}/notifications/devices`],
    ['DELETE', `${base}/immich/config`],
    ['patch', `${base}/users/language`],
    ['PATCH', '/api/v1/users/date-format'],
  ])('flags %s %s', (method, url) => {
    expect(isSettingsMutation(method, url)).toBe(true);
  });

  it.each([
    ['GET', `${base}/users/enabled-contact-fields`],
    ['GET', `${base}/notifications/config`],
    ['HEAD', `${base}/users/date-format`],
    ['POST', `${base}/contacts`],
    ['PATCH', `${base}/contacts/12`],
    ['POST', `${base}/users/directory`],
    ['POST', `${base}/login`],
    ['PUT', 'http://localhost:7300/not/api/v1/users/language'],
    ['PATCH', 'http://[::bad'],
  ])('does not flag %s %s', (method, url) => {
    expect(isSettingsMutation(method, url)).toBe(false);
  });
});

describe('spec auth wiring', () => {
  // Specs that import `test` from '@playwright/test' directly bypass the
  // per-worker storageState override, so with no storageState in the project
  // config they'd run logged out. backupRestore drives its own API contexts and
  // never touches the `page`/`request` fixtures, so it is the one exception.
  const RAW_TEST_ALLOWLIST = new Set(['backupRestore.spec.ts']);

  it('every spec takes `test` from ./fixtures (not @playwright/test)', () => {
    const offenders = fs
      .readdirSync(E2E_DIR)
      .filter((f) => f.endsWith('.spec.ts') && !RAW_TEST_ALLOWLIST.has(f))
      .filter((f) => {
        const src = fs.readFileSync(path.join(E2E_DIR, f), 'utf8');
        const imports = [...src.matchAll(/import\s*\{([^}]*)\}\s*from\s*'@playwright\/test'/g)];
        return imports.some((m) =>
          m[1].split(',').some((n) => /^\s*(test|test\s+as\s+\w+)\s*$/.test(n)),
        );
      });
    expect(offenders).toEqual([]);
  });

  it('the allowlisted exception really does not use the page/request fixtures', () => {
    const src = fs.readFileSync(path.join(E2E_DIR, 'backupRestore.spec.ts'), 'utf8');
    expect(src).not.toMatch(/async \(\{[^}]*\b(page|request)\b[^}]*\}\)/);
  });

  it('the settings lock is gone for good', () => {
    const fixtures = fs.readFileSync(path.join(E2E_DIR, 'fixtures.ts'), 'utf8');
    expect(fixtures).not.toMatch(/USER_SETTINGS_LOCK|withExclusiveUserSettings/);
  });
});
