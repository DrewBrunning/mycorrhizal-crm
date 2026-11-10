import { describe, expect, test } from 'vitest';
import { formatLinkPeriod, isLinkCurrent, periodFromYears, yearsFromPeriod } from './entityPeriod';

const today = { year: 2026, month: 11, day: 9 };

describe('isLinkCurrent', () => {
  test('no period is always current', () => {
    expect(isLinkCurrent(undefined, today)).toBe(true);
    expect(isLinkCurrent({}, today)).toBe(true);
  });
  test('a past end or a future start is not current', () => {
    expect(isLinkCurrent({ end: { year: 2020 } }, today)).toBe(false);
    expect(isLinkCurrent({ start: { year: 2030 } }, today)).toBe(false);
    expect(isLinkCurrent({ start: { year: 2026, month: 12 } }, today)).toBe(false);
  });
  test('dates that include today, or cannot be compared, are current', () => {
    expect(isLinkCurrent({ start: { year: 2019 }, end: { year: 2026 } }, today)).toBe(true);
    // year-only end in the current year: same year is not "past" (ADR 0015 rule 3).
    expect(isLinkCurrent({ end: { year: 2026, month: 11 } }, today)).toBe(true);
    expect(isLinkCurrent({ start: { year: 2026, month: 11, day: 9 } }, today)).toBe(true);
  });
});

describe('years <-> period', () => {
  test('periodFromYears builds whole-year endpoints and an empty one is undefined', () => {
    expect(periodFromYears('2019', '')).toEqual({ start: { year: 2019 } });
    expect(periodFromYears('', '2021')).toEqual({ end: { year: 2021 } });
    expect(periodFromYears('', '')).toBeUndefined();
    expect(periodFromYears('2019', '2021')).toEqual({ start: { year: 2019 }, end: { year: 2021 } });
  });
  test('periodFromYears keeps a finer existing endpoint when the year is unchanged', () => {
    const existing = { start: { year: 2019, month: 4, day: 2 } };
    expect(periodFromYears('2019', '', existing)).toEqual({
      start: { year: 2019, month: 4, day: 2 },
    });
    expect(periodFromYears('2020', '', existing)).toEqual({ start: { year: 2020 } });
  });
  test('yearsFromPeriod', () => {
    expect(yearsFromPeriod({ start: { year: 2019 }, end: { year: 2021 } })).toEqual({
      startYear: '2019',
      endYear: '2021',
    });
    expect(yearsFromPeriod(undefined)).toEqual({ startYear: '', endYear: '' });
  });
  test('invalid year text is rejected', () => {
    expect(periodFromYears('20x9', '')).toBe('invalid');
    expect(periodFromYears('2019', '2018')).toBe('invalid');
  });
});

describe('formatLinkPeriod', () => {
  const t = (k: string, o?: Record<string, unknown>) => `${k}:${JSON.stringify(o ?? {})}`;
  test('range, since, until and none', () => {
    expect(formatLinkPeriod({ start: { year: 2019 }, end: { year: 2021 } }, t)).toBe('2019 – 2021');
    expect(formatLinkPeriod({ start: { year: 2019 } }, t)).toContain('since');
    expect(formatLinkPeriod({ end: { year: 2021 } }, t)).toContain('until');
    expect(formatLinkPeriod(undefined, t)).toBe('');
  });
});
