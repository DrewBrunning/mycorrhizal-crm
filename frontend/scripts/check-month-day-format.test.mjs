// biome-ignore-all lint/suspicious/noTemplateCurlyInString: fixtures contain the literal template-literal source under test.
// @vitest-environment node
//
// Tests for check-month-day-format.mjs (issue #2920, work item 3), the guard
// that fails when a `.tsx` file assembles a month and day by hand instead of
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

  test('only scans .tsx files (the scope of the ESLint rule)', () => {
    expect(findOffenders([file('src/util.ts', 'const x = `${month}-${day}`;')])).toEqual([]);
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
  });
});
