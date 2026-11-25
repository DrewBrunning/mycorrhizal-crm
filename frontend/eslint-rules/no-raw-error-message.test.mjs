// @vitest-environment node
//
// Tests for local/no-raw-error-message (issue #2920 item 1). The rule is
// type-aware, so each case is linted through a real typescript-eslint program
// (a default project over a temp file), not a parse-only RuleTester.
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { ESLint } from 'eslint';
import tseslint from 'typescript-eslint';
import { afterAll, beforeAll, describe, expect, test } from 'vitest';
import rule from './no-raw-error-message.js';

let dir;
let eslint;

beforeAll(() => {
  dir = fs.mkdtempSync(path.join(os.tmpdir(), 'no-raw-error-message-'));
  eslint = new ESLint({
    cwd: dir,
    overrideConfigFile: true,
    overrideConfig: [
      {
        files: ['**/*.ts'],
        languageOptions: {
          parser: tseslint.parser,
          parserOptions: {
            projectService: { allowDefaultProject: ['*.ts'] },
            tsconfigRootDir: dir,
          },
        },
        plugins: { local: { rules: { 'no-raw-error-message': rule } } },
        rules: { 'local/no-raw-error-message': 'error' },
      },
    ],
  });
});

afterAll(() => {
  fs.rmSync(dir, { recursive: true, force: true });
});

const PRELUDE = `
declare function show(text: string): void;
declare function t(key: string): string;
declare function getErrorMessage(e: unknown): string;
declare function logError(e: unknown, ctx: object): void;
class ApiError extends Error { code = 'X'; }
`;

/** Lints `body` (after the prelude) and returns the reported lines' source text. */
async function reported(body) {
  // One file name, re-linted with new text: the default project caps how many
  // distinct files it will serve.
  const file = path.join(dir, 'case.ts');
  const source = `${PRELUDE}${body}\n`;
  fs.writeFileSync(file, source);
  const [result] = await eslint.lintText(source, { filePath: file });
  const fatal = result.messages.filter((m) => m.fatal || !m.ruleId);
  expect(fatal, JSON.stringify(fatal)).toEqual([]);
  const lines = source.split('\n');
  return result.messages.map((m) => lines[m.line - 1].trim());
}

describe('reports a raw Error.message used as UI text', () => {
  test.each([
    [
      'the original ternary',
      `try {} catch (err) { show(err instanceof Error ? err.message : t('x')); }`,
    ],
    [
      'an && guard in the test (#2920 follow-up shape)',
      `try {} catch (err) { show(err instanceof Error && err.message ? err.message : t('x')); }`,
    ],
    ['an if-block', `try {} catch (err) { if (err instanceof Error) { show(err.message); } }`],
    ['a typed callback parameter', `const onError = (error: Error) => show(error.message);`],
    ['an Error subclass (ApiError)', `const f = (e: ApiError) => show(e.message);`],
    ['a DOMException', `const f = (e: DOMException) => show(e.message);`],
    ['an optional chain', `const f = (e?: Error) => show(e?.message ?? t('x'));`],
    ['a || fallback', `const f = (e: Error) => show(e.message || t('x'));`],
    ['an `as Error` cast', `try {} catch (err) { show((err as Error).message); }`],
    ['a returned value', `function f(e: Error): string { return e.message; }`],
  ])('%s', async (_name, body) => {
    const hits = await reported(body);
    expect(hits).toHaveLength(1);
  });
});

describe('allows reads that only inspect the message', () => {
  test.each([
    [
      'a comparison through a type assertion',
      `const f = (e: Error) => (e.message as string) === 'x';`,
    ],
    [
      'a comparison',
      `try {} catch (err) { if (err instanceof Error && err.message === 'death') show(t('a')); }`,
    ],
    ['a method call on it', `const f = (e: Error) => e.message.includes('denied');`],
    ['a Set lookup', `const S = new Set<string>(); const f = (e: Error) => S.has(e.message);`],
    ['console output', `const f = (e: Error) => console.error(e.message);`],
    ['logError', `const f = (e: Error) => logError(e.message, {});`],
    ['re-wrapping in a new Error', `const f = (e: Error) => { throw new Error(e.message); };`],
    ['a truthiness test only', `const f = (e: Error) => (e.message ? t('a') : t('b'));`],
    [
      'getErrorMessage',
      `try {} catch (err) { show(err instanceof Error ? getErrorMessage(err) : t('x')); }`,
    ],
    ['a non-Error object', `const f = (r: { message: string }) => show(r.message);`],
  ])('%s', async (_name, body) => {
    expect(await reported(body)).toEqual([]);
  });
});
