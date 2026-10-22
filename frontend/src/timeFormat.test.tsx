import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { setDateFormatPreference } from './DateFormatProvider';
import {
  formatDateTime,
  formatDateTimeWith,
  formatLocalDate,
  formatPreferredDateTime,
  formatTime,
  formatTimeWith,
  getTimeFormat,
  initializeTimeFormatFromBackend,
  isTimeFormat,
  setTimeFormat,
  TIME_FORMATS,
  timeFormatOptions,
  useFormatDateTime,
  useTimeFormat,
} from './timeFormat';

afterEach(cleanup);

beforeEach(() => {
  window.localStorage.clear();
  setTimeFormat('auto');
  setDateFormatPreference('eu');
});

// 15:30 UTC on 4 July 2026, rendered in UTC so the test is zone-independent.
const AFTERNOON = new Date(Date.UTC(2026, 6, 4, 15, 30));
const JUST_AFTER_MIDNIGHT = new Date(Date.UTC(2026, 6, 4, 0, 5));
const UTC = { timeZone: 'UTC' } as const;

// ICU separates the time from AM/PM with a (narrow) no-break space on newer
// versions; normalise so the assertions don't depend on the ICU build.
const norm = (s: string) => s.replace(/\s/g, ' ');
const time = (tf: 'auto' | '12h' | '24h', locale: string, d: Date = AFTERNOON) =>
  norm(formatTimeWith(d, tf, locale, { hour: 'numeric', minute: '2-digit', ...UTC }));

describe('timeFormatOptions', () => {
  test('auto leaves hourCycle unset (the locale default)', () => {
    expect(timeFormatOptions('auto')).toEqual({});
    expect('hourCycle' in timeFormatOptions('auto')).toBe(false);
  });
  test('12h and 24h set h12 / h23', () => {
    expect(timeFormatOptions('12h')).toEqual({ hourCycle: 'h12' });
    expect(timeFormatOptions('24h')).toEqual({ hourCycle: 'h23' });
  });
});

describe('formatTimeWith', () => {
  test('en-US: auto is 12-hour, 24h overrides', () => {
    expect(time('auto', 'en-US')).toBe('3:30 PM');
    expect(time('12h', 'en-US')).toBe('3:30 PM');
    expect(time('24h', 'en-US')).toBe('15:30');
  });
  test('de: auto is 24-hour, 12h overrides', () => {
    expect(time('auto', 'de')).toBe('15:30');
    expect(time('24h', 'de')).toBe('15:30');
    expect(time('12h', 'de')).toMatch(/^0?3:30 PM$/);
  });
  test('en-GB: auto is 24-hour, 12h overrides', () => {
    expect(time('auto', 'en-GB')).toBe('15:30');
    expect(time('24h', 'en-GB')).toBe('15:30');
    expect(time('12h', 'en-GB')).toMatch(/^0?3:30 pm$/i);
  });
  test('24h renders just after midnight as 00:05, never 24:05', () => {
    expect(time('24h', 'en-US', JUST_AFTER_MIDNIGHT)).toBe('00:05');
    expect(time('12h', 'en-US', JUST_AFTER_MIDNIGHT)).toBe('12:05 AM');
  });
  test('defaults to hour + minute and accepts a string or number', () => {
    expect(formatTimeWith(AFTERNOON.toISOString(), '24h', 'en-US')).toMatch(/^\d{2}:\d{2}$/);
    expect(formatTimeWith(AFTERNOON.getTime(), '24h', 'en-US')).toMatch(/^\d{2}:\d{2}$/);
  });
  test('an unparseable value is returned as-is', () => {
    expect(formatTimeWith('not a time', '24h', 'en-US')).toBe('not a time');
  });
});

describe('formatDateTimeWith', () => {
  test('keeps the date part and applies the hour cycle', () => {
    const base = { year: 'numeric', month: 'numeric', day: 'numeric', ...UTC } as const;
    const opts = { ...base, hour: 'numeric', minute: '2-digit' } as const;
    expect(norm(formatDateTimeWith(AFTERNOON, '24h', 'en-US', opts))).toBe('7/4/2026, 15:30');
    expect(norm(formatDateTimeWith(AFTERNOON, '12h', 'en-US', opts))).toBe('7/4/2026, 3:30 PM');
    expect(norm(formatDateTimeWith(AFTERNOON, 'auto', 'de', opts))).toBe('4.7.2026, 15:30');
    expect(norm(formatDateTimeWith(AFTERNOON, '12h', 'de', opts))).toMatch(
      /^4\.7\.2026, 0?3:30 PM$/,
    );
    expect(norm(formatDateTimeWith(AFTERNOON, 'auto', 'en-GB', opts))).toBe('04/07/2026, 15:30');
  });
  test('with no options it renders date + time (toLocaleString parity)', () => {
    const out = formatDateTimeWith(AFTERNOON, '24h', 'en-US', undefined);
    expect(out).toBe(AFTERNOON.toLocaleString('en-US', { hourCycle: 'h23' }));
    expect(out).toMatch(/\d{1,2}:\d{2}:\d{2}/);
    expect(out).not.toMatch(/AM|PM/);
  });
  test('an unparseable value is returned as-is', () => {
    expect(formatDateTimeWith('nope', 'auto', 'en-US')).toBe('nope');
  });
});

describe('the current preference', () => {
  test('is an enum mirror of the backend values', () => {
    expect([...TIME_FORMATS]).toEqual(['auto', '12h', '24h']);
    expect(isTimeFormat('12h')).toBe(true);
    expect(isTimeFormat('13h')).toBe(false);
    expect(isTimeFormat(undefined)).toBe(false);
  });

  test('defaults to auto and persists a change to localStorage', () => {
    expect(getTimeFormat()).toBe('auto');
    setTimeFormat('24h');
    expect(getTimeFormat()).toBe('24h');
    expect(window.localStorage.getItem('timeFormat')).toBe('24h');
  });

  test('ignores a value outside the enum', () => {
    setTimeFormat('24h');
    setTimeFormat('13h' as never);
    expect(getTimeFormat()).toBe('24h');
  });

  test('initializeTimeFormatFromBackend applies a valid value only', () => {
    initializeTimeFormatFromBackend('12h');
    expect(getTimeFormat()).toBe('12h');
    initializeTimeFormatFromBackend('bogus');
    initializeTimeFormatFromBackend(undefined);
    expect(getTimeFormat()).toBe('12h');
  });

  test('module-level formatters read the current preference', () => {
    setTimeFormat('24h');
    expect(formatTime(AFTERNOON, { hour: 'numeric', minute: '2-digit', ...UTC })).toMatch(
      /^15:30$/,
    );
    expect(formatDateTime(AFTERNOON, { hour: 'numeric', minute: '2-digit', ...UTC })).toMatch(
      /^15:30$/,
    );
    setTimeFormat('12h');
    expect(norm(formatTime(AFTERNOON, { hour: 'numeric', minute: '2-digit', ...UTC }))).toMatch(
      /^3:30 PM$/i,
    );
  });

  test('useTimeFormat re-renders on change and binds the formatters', () => {
    const { result } = renderHook(() => useTimeFormat());
    expect(result.current.timeFormat).toBe('auto');
    act(() => result.current.setTimeFormat('24h'));
    expect(result.current.timeFormat).toBe('24h');
    expect(
      result.current.formatTime(AFTERNOON, { hour: 'numeric', minute: '2-digit', ...UTC }),
    ).toBe('15:30');
    expect(
      result.current.formatDateTime(AFTERNOON, { hour: 'numeric', minute: '2-digit', ...UTC }),
    ).toBe('15:30');
    act(() => result.current.setTimeFormat('12h'));
    expect(
      norm(result.current.formatTime(AFTERNOON, { hour: 'numeric', minute: '2-digit', ...UTC })),
    ).toBe('3:30 PM');
  });

  test('setting the same value again does not notify', () => {
    const { result } = renderHook(() => useTimeFormat());
    act(() => result.current.setTimeFormat('auto'));
    expect(result.current.timeFormat).toBe('auto');
  });
});

describe('formatPreferredDateTime', () => {
  // Built from local parts so the test does not depend on the process zone.
  const local = new Date(2026, 9, 26, 18, 6, 42);

  test('uses the date-format preference plus a locale time with no seconds', () => {
    expect(formatPreferredDateTime(local, 'eu', '24h', 'en-US')).toBe('26.10.2026 18:06');
    expect(formatPreferredDateTime(local, 'us', '24h', 'en-US')).toBe('10/26/2026 18:06');
    expect(formatPreferredDateTime(local, 'iso', '24h', 'en-US')).toBe('2026-10-26 18:06');
    expect(formatPreferredDateTime(local, 'us-mmmm', '24h', 'en-US')).toBe(
      'October 26, 2026 18:06',
    );
  });

  test('honours the 12-hour preference', () => {
    const out = norm(formatPreferredDateTime(local, 'eu', '12h', 'en-US'));
    expect(out).toBe('26.10.2026 6:06 PM');
  });

  test('never renders seconds', () => {
    const out = formatPreferredDateTime(local, 'eu', '24h', 'en-US');
    expect(out).not.toMatch(/\d{2}:\d{2}:\d{2}/);
    expect(out.match(/\d{2}:\d{2}$/)?.[0]).toBe('18:06');
  });

  test('returns an unparseable value as-is', () => {
    expect(formatPreferredDateTime('not a date', 'eu', '24h', 'en-US')).toBe('not a date');
  });
});

describe('formatLocalDate', () => {
  test('renders a timestamp as a date in the preference, with no time', () => {
    const local = new Date(2026, 9, 26, 18, 6, 42);
    expect(formatLocalDate(local, 'eu')).toBe('26.10.2026');
    expect(formatLocalDate(local, 'us')).toBe('10/26/2026');
    expect(formatLocalDate(local, 'iso')).toBe('2026-10-26');
    expect(formatLocalDate('not a date', 'eu')).toBe('not a date');
  });
});

describe('the module-level timestamp formatter', () => {
  test('reads both preferences, so no caller can leak a US date or seconds', () => {
    const local = new Date(2026, 9, 26, 18, 6, 42);
    setDateFormatPreference('us');
    setTimeFormat('24h');
    expect(formatDateTime(local)).toBe('10/26/2026 18:06');
    setDateFormatPreference('eu');
    setTimeFormat('12h');
    expect(norm(formatDateTime(local))).toBe('26.10.2026 6:06 PM');
  });
});

describe('useFormatDateTime', () => {
  test('re-renders when the date or time preference changes', () => {
    const local = new Date(2026, 9, 26, 18, 6, 42);
    setTimeFormat('24h');
    const { result } = renderHook(() => useFormatDateTime());
    expect(result.current(local)).toBe('26.10.2026 18:06');

    act(() => setDateFormatPreference('us'));
    expect(result.current(local)).toBe('10/26/2026 18:06');

    act(() => setTimeFormat('12h'));
    expect(norm(result.current(local))).toBe('10/26/2026 6:06 PM');
  });
});
