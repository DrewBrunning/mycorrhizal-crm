import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, test } from 'vitest';

// Stryker runs vitest from a copy of frontend/ at .stryker-tmp/sandbox-*/, so a
// `../../../testdata/...` path written relative to a test file resolves to
// nothing there. Stryker 10.0.0 silently dropped such test files from the
// mutation run; 10.1.0 fails the dry run. Shared repo-root fixtures must go
// through the `@testdata` alias, which vitest.config.ts resolves by walking up
// to the real testdata/ from either location.

const SRC_ROOT = join(process.cwd(), 'src');
const RELATIVE_TESTDATA = /['"`](?:\.\.\/)+testdata\//;

function collectSourceFiles(dir: string, acc: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      collectSourceFiles(full, acc);
    } else if (/\.(ts|tsx)$/.test(entry)) {
      acc.push(full);
    }
  }
  return acc;
}

describe('repo-root testdata access', () => {
  test('no src file reaches testdata/ through a relative path', () => {
    const offenders = collectSourceFiles(SRC_ROOT)
      .filter((file) => !file.endsWith('testdataAlias.test.ts'))
      .filter((file) => RELATIVE_TESTDATA.test(readFileSync(file, 'utf8')));
    expect(offenders).toEqual([]);
  });
});
