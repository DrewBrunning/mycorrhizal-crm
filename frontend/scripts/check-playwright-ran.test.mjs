// @vitest-environment node
//
// Tests for check-playwright-ran.mjs: the guard that fails a Playwright run
// which "passed" by executing nothing. Playwright exits 0 when every test
// self-skips, so a lost RUN_A11Y_SWEEP=1 would otherwise leave the nightly
// populated a11y sweep (#2934) silently green.

import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { describe, expect, test } from 'vitest';
import { checkRan, main } from './check-playwright-ran.mjs';

const report = (stats) => JSON.stringify({ stats });

describe('checkRan', () => {
  test('passes when enough tests actually ran and none failed', () => {
    expect(checkRan(report({ expected: 108, skipped: 0, unexpected: 0, flaky: 0 }), 40)).toEqual({
      ok: true,
      message: '108 tests ran (floor 40)',
    });
  });

  test('fails when every test self-skipped (the lost-env-var shape)', () => {
    const result = checkRan(report({ expected: 0, skipped: 108, unexpected: 0, flaky: 0 }), 40);
    expect(result.ok).toBe(false);
    expect(result.message).toMatch(/ran 0 tests.*108 skipped/);
  });

  test('fails when fewer tests ran than the floor', () => {
    expect(checkRan(report({ expected: 3, skipped: 0, unexpected: 0, flaky: 0 }), 40).ok).toBe(
      false,
    );
  });

  test('flaky tests count as having run', () => {
    expect(checkRan(report({ expected: 39, skipped: 0, unexpected: 0, flaky: 1 }), 40).ok).toBe(
      true,
    );
  });

  test('rejects a report with no stats object', () => {
    expect(checkRan('{}', 1).ok).toBe(false);
    expect(checkRan('not json', 1).ok).toBe(false);
  });
});

describe('main', () => {
  const write = (body) => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'pwran-'));
    const file = path.join(dir, 'results.json');
    fs.writeFileSync(file, body);
    return file;
  };

  test('exit code 1 for an all-skipped report, 0 for a real one', () => {
    const quiet = { log: () => {}, error: () => {} };
    expect(
      main(
        [write(report({ expected: 0, skipped: 9, unexpected: 0, flaky: 0 })), '--min', '1'],
        quiet,
      ),
    ).toBe(1);
    expect(
      main(
        [write(report({ expected: 9, skipped: 0, unexpected: 0, flaky: 0 })), '--min', '1'],
        quiet,
      ),
    ).toBe(0);
  });

  test('exit code 1 when the report file is missing', () => {
    const quiet = { log: () => {}, error: () => {} };
    expect(main(['/nonexistent/results.json', '--min', '1'], quiet)).toBe(1);
  });
});
