// biome-ignore-all lint/suspicious/noTemplateCurlyInString: fixtures contain the literal template-literal source under test.
// @vitest-environment node
//
// Tests for check-month-day-format.mjs (issue #2920, work item 3), the guard
// that fails when a `.ts`/`.tsx` file assembles a month and day by hand instead of
// calling formatMonthDay(). The last describe runs it against the REAL repo,
// so `vitest run` (the required "Frontend (Vitest)" job) enforces it with no
// workflow edit.

import { describe, expect, test } from 'vitest';
import {
  compareAllowlist,
  findOffenders,
  loadAllowlist,
  loadSourceFiles,
} from './check-month-day-format.mjs';

const LONG_REASON = 'a reason that is long enough';

const file = (name, source) => ({ file: name, source });

describe('findOffenders', () => {
  test('flags a template literal interpolating month next to day', () => {
    const [offender] = findOffenders([
      file('src/Probe.tsx', 'export const x = `${month} ${day}`;'),
    ]);
    expect(offender).toMatchObject({ file: 'src/Probe.tsx', line: 1 });
    expect(offender.text).toBe('`${month} ${day}`');
  });

  test('flags a nested template literal and a call around the month', () => {
    const source =
      'const anchorLabel = (o: O) => `${t(`occasions.months.${o.anchor_month}`)} ${o.anchor_day}`;';
    expect(findOffenders([file('src/Occasion.tsx', source)])).toHaveLength(1);
  });

  test('does not require a space between the parts', () => {
    expect(findOffenders([file('src/Probe.tsx', 'const x = `${month}/${day}`;')])).toHaveLength(1);
  });

  test('ignores month-only, day-only and non-interpolated text', () => {
    const sources = [
      file('src/A.tsx', 'const x = `${month}`;'),
      file('src/B.tsx', 'const x = `${day}`;'),
      file('src/C.tsx', 'const x = "month day";'),
      file('src/D.tsx', 'const x = formatMonthDay(month, day, locale);'),
    ];
    expect(findOffenders(sources)).toEqual([]);
  });

  test('exempts DateFormatProvider.tsx, which owns the formatters', () => {
    const source = 'const x = `${month}/${day}/${year}`;';
    expect(findOffenders([file('src/DateFormatProvider.tsx', source)])).toEqual([]);
  });

  test('scans .ts helpers too (the #2908 date came from one)', () => {
    expect(findOffenders([file('src/utils/x.ts', 'const x = `${month}/${day}`;')])).toHaveLength(1);
  });

  test('flags the Date accessors, not only identifiers named month/day', () => {
    const source = 'const x = `${d.getMonth() + 1}/${d.getDate()}`;';
    expect(findOffenders([file('src/Probe.tsx', source)])).toHaveLength(1);
    const utc = 'const x = `${d.getUTCDate()}. ${d.getUTCMonth() + 1}.`;';
    expect(findOffenders([file('src/Probe.ts', utc)])).toHaveLength(1);
  });

  test('ignores machine ISO shapes (input values and API parameters)', () => {
    const sources = [
      file('src/A.ts', 'const x = `${year}-${month}-${day}`;'),
      file('src/B.ts', 'const x = `--${month}-${day}`;'),
      file('src/C.tsx', 'const x = `${y}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${h}:${m}`;'),
    ];
    expect(findOffenders(sources)).toEqual([]);
  });

  test('still flags an ISO-looking template with display text around it', () => {
    const source = 'const x = `Due ${year}-${month}-${day}`;';
    expect(findOffenders([file('src/Probe.tsx', source)])).toHaveLength(1);
  });

  test('skips declaration files', () => {
    expect(loadSourceFiles().some(({ file: f }) => f.endsWith('.d.ts'))).toBe(false);
  });

  test('reports the one-based line number', () => {
    const source = 'const a = 1;\nconst b = 2;\nconst x = `${month} ${day}`;';
    expect(findOffenders([file('src/Probe.tsx', source)])[0].line).toBe(3);
  });
});

describe('compareAllowlist', () => {
  const offenders = [
    { file: 'src/A.tsx', line: 1, text: '`${month} ${day}`' },
    { file: 'src/B.tsx', line: 2, text: '`${month}/${day}`' },
  ];

  test('passes when every offender is covered by the exact count', () => {
    const result = compareAllowlist(offenders, {
      files: {
        'src/A.tsx': { count: 1, reason: LONG_REASON },
        'src/B.tsx': { count: 1, reason: LONG_REASON },
      },
    });
    expect(result).toMatchObject({ ok: true, added: [], stale: [], badReasons: [] });
  });

  test('reports a new offender in an unlisted file', () => {
    const result = compareAllowlist(offenders, {
      files: { 'src/A.tsx': { count: 1, reason: LONG_REASON } },
    });
    expect(result.ok).toBe(false);
    expect(result.added).toEqual([expect.stringContaining('src/B.tsx: found 1')]);
  });

  test('reports a new offender when an allowlisted file gains one', () => {
    const result = compareAllowlist(offenders, {
      files: {
        'src/A.tsx': { count: 0, reason: LONG_REASON },
        'src/B.tsx': { count: 1, reason: LONG_REASON },
      },
    });
    expect(result.added).toEqual([expect.stringContaining('src/A.tsx: found 1, allowlist has 0')]);
  });

  test('reports a stale entry when the site was removed', () => {
    const result = compareAllowlist(offenders, {
      files: {
        'src/A.tsx': { count: 1, reason: LONG_REASON },
        'src/B.tsx': { count: 1, reason: LONG_REASON },
        'src/Gone.tsx': { count: 1, reason: LONG_REASON },
      },
    });
    expect(result.ok).toBe(false);
    expect(result.stale).toEqual([
      expect.stringContaining('src/Gone.tsx: allowlist has 1, found 0'),
    ]);
  });

  test('reports a stale count when the allowlist is higher than the tree', () => {
    const result = compareAllowlist(offenders.slice(0, 1), {
      files: { 'src/A.tsx': { count: 2, reason: LONG_REASON } },
    });
    expect(result.stale).toEqual([expect.stringContaining('src/A.tsx: allowlist has 2, found 1')]);
  });

  test('rejects an entry with no real reason or a bad count', () => {
    const result = compareAllowlist([], {
      files: {
        'src/A.tsx': { count: 0, reason: 'todo' },
        'src/B.tsx': { count: -1, reason: LONG_REASON },
      },
    });
    expect(result.ok).toBe(false);
    expect(result.badReasons).toHaveLength(2);
  });
});

describe('the real repository', () => {
  test('every hand-assembled month/day is counted in month-day-allowlist.json', () => {
    const offenders = findOffenders(loadSourceFiles());
    const result = compareAllowlist(offenders, loadAllowlist());
    expect(result).toMatchObject({ ok: true, added: [], stale: [], badReasons: [] });
    // Parses every source file with the TypeScript compiler (~10 s on a loaded
    // nightly runner), so it needs more than the 10 s global testTimeout.
  }, 60_000);
});

describe('the ESLint selector and this script agree', () => {
  // The lint rule (eslint.config.js) and this script are two encodings of one
  // rule; a probe table run through both keeps them from drifting.
  const probes = [
    ['`${month} ${day}`', true],
    ['`${month}/${day}`', true],
    ['`${d.getMonth() + 1}/${d.getDate()}`', true],
    ['`${d.getUTCDate()}. ${d.getUTCMonth() + 1}.`', true],
    ['`Due ${year}-${month}-${day}`', true],
    ['`${year}-${month}-${day}`', false],
    ['`--${month}-${day}`', false],
    ['`${y}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${h}:${m}`', false],
    ['`${month}`', false],
    ['`${day} ${weekday}`', false],
  ];

  test.each(probes)('%s -> flagged: %s', async (expr, flagged) => {
    const { Linter } = await import('eslint');
    const tseslint = (await import('typescript-eslint')).default;
    const { monthDaySelector } = await import('../eslint.config.js');
    const code = `declare const d: Date; declare const month: number, day: number, year: number, weekday: string, y: number, h: number, m: number; declare function pad(n: number): string;\nexport const x = ${expr};\n`;
    const messages = new Linter({ configType: 'flat' }).verify(
      code,
      [
        {
          files: ['**/*.ts'],
          languageOptions: { parser: tseslint.parser },
          rules: { 'no-restricted-syntax': ['error', monthDaySelector] },
        },
      ],
      'probe.ts',
    );
    expect(messages.filter((msg) => msg.fatal)).toEqual([]);
    expect(messages.length > 0).toBe(flagged);
    expect(findOffenders([file('src/probe.ts', code)]).length > 0).toBe(flagged);
  });
});
