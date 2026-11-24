// @vitest-environment node
//
// Tests for check-raw-error-message.mjs (issue #2920 item 1), the counted
// ratchet for the `err instanceof Error ? err.message : ...` pattern. The last
// describe runs against the REAL repo, so `vitest run` enforces it with no
// workflow edit (the workflow also runs the script directly).

import * as fs from 'node:fs';
import { describe, expect, test } from 'vitest';
import {
  ALLOWLIST_FILE,
  checkRawErrorMessages,
  countRawErrorMessages,
  loadAllowlist,
  scanRawErrorMessages,
} from './check-raw-error-message.mjs';

const reason = 'a reason that is long enough to pass';

describe('countRawErrorMessages', () => {
  test('counts a same-line and a multi-line conditional', () => {
    expect(countRawErrorMessages("const m = err instanceof Error ? err.message : t('x');")).toBe(1);
    expect(
      countRawErrorMessages(`
        setError(
          err instanceof Error
            ? err.message
            : t('x'),
        );`),
    ).toBe(1);
  });

  test('counts every matching site in a file', () => {
    const source = `
      const a = err instanceof Error ? err.message : t('a');
      const b = error instanceof Error ? error.message : t('b');
      const c = e instanceof Error ? e.message : '';`;
    expect(countRawErrorMessages(source)).toBe(3);
  });

  test('ignores a logical guard, an if-statement and a different consequent', () => {
    expect(
      countRawErrorMessages(`
        const m = err instanceof Error && err.message ? err.message : t('x');
        if (err instanceof Error) return err.message;
        const n = err instanceof Error ? t('x') : err.message;`),
    ).toBe(0);
  });

  test('ignores a check whose right side is not the Error constructor', () => {
    expect(countRawErrorMessages('const m = err instanceof MyError ? err.message : t("x");')).toBe(
      0,
    );
  });
});

describe('checkRawErrorMessages', () => {
  test('passes on an exact match', () => {
    expect(checkRawErrorMessages({ 'src/a.ts': 2 }, { 'src/a.ts': { count: 2, reason } })).toEqual({
      ok: true,
      unexpected: [],
      increased: [],
      stale: [],
      badReasons: [],
    });
  });

  test('reports a new file that is not allowlisted', () => {
    const r = checkRawErrorMessages({ 'src/a.ts': 1 }, {});
    expect(r.ok).toBe(false);
    expect(r.unexpected).toEqual(['src/a.ts (1 site)']);
  });

  test('reports a file that gained a site', () => {
    const r = checkRawErrorMessages({ 'src/a.ts': 3 }, { 'src/a.ts': { count: 2, reason } });
    expect(r.ok).toBe(false);
    expect(r.increased).toEqual(['src/a.ts (3 sites, allowlist has 2)']);
  });

  test('reports a stale count (a site was removed) and a stale file entry', () => {
    const lower = checkRawErrorMessages({ 'src/a.ts': 1 }, { 'src/a.ts': { count: 2, reason } });
    expect(lower.stale).toHaveLength(1);
    expect(lower.ok).toBe(false);

    const gone = checkRawErrorMessages({}, { 'src/a.ts': { count: 2, reason } });
    expect(gone.stale).toHaveLength(1);
    expect(gone.ok).toBe(false);
  });

  test('rejects an entry with no real reason or a bad count', () => {
    const r = checkRawErrorMessages(
      { 'src/a.ts': 1 },
      { 'src/a.ts': { count: 1, reason: 'todo' }, 'src/b.ts': { count: 0, reason } },
    );
    expect(r.badReasons).toEqual(['src/a.ts', 'src/b.ts']);
    expect(r.ok).toBe(false);
  });
});

describe('the real repository', () => {
  test('every current site is covered by the committed count', () => {
    const actual = scanRawErrorMessages();
    const allowlist = loadAllowlist(ALLOWLIST_FILE);
    const result = checkRawErrorMessages(actual, allowlist);
    expect(result).toMatchObject({
      ok: true,
      unexpected: [],
      increased: [],
      stale: [],
      badReasons: [],
    });
    expect(Object.values(actual).reduce((sum, count) => sum + count, 0)).toBeGreaterThan(0);
  });
});

test('the allowlist file is valid JSON with a reason per entry', () => {
  const entries = Object.entries(loadAllowlist(ALLOWLIST_FILE));
  expect(entries.length).toBeGreaterThan(0);
  for (const [file, entry] of entries) {
    expect(file.startsWith('src/')).toBe(true);
    expect(entry.reason.trim().length).toBeGreaterThanOrEqual(15);
  }
  expect(fs.existsSync(ALLOWLIST_FILE)).toBe(true);
});
