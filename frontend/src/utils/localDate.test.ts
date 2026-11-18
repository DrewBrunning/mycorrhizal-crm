import { afterEach, describe, expect, test, vi } from 'vitest';
import { formatDateWithFormat } from '../DateFormatProvider';
import { useTimeZone } from '../testSupport/timeZone';
import {
  calendarDateOnly,
  isBeforeLocalToday,
  localDaysUntil,
  localISODate,
  todayLocalISODate,
} from './localDate';

// useTimeZone simulates the process zone on Date's local getters. Runtime TZ
// environment mutation (the old technique here) is ignored in worker threads,
// and Stryker's runner hard-codes `pool: 'threads'` -- see the helper and
// issue #1843.

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe('todayLocalISODate (#1727)', () => {
  test('west of UTC in the evening, today is the local date, not the UTC date', () => {
    useTimeZone('America/Chicago'); // CDT, UTC-05:00 on this date
    // 22:30 local on 11 March == 03:30Z on 12 March.
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-03-12T03:30:00Z'));
    expect(todayLocalISODate()).toBe('2026-03-11');
  });

  test('east of UTC in the morning, today is the local date, not the UTC date', () => {
    useTimeZone('Pacific/Auckland'); // NZST, UTC+12 in June
    // 08:00 local on 12 June == 20:00Z on 11 June.
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-06-11T20:00:00Z'));
    expect(todayLocalISODate()).toBe('2026-06-12');
  });

  test('accepts an injected clock and zero-pads', () => {
    useTimeZone('UTC');
    expect(todayLocalISODate(new Date('2026-01-02T12:00:00Z'))).toBe('2026-01-02');
    expect(localISODate(new Date('2026-01-05T12:00:00Z'))).toBe('2026-01-05');
  });
});

describe('calendarDateOnly (#1778)', () => {
  test('rewrites an exact UTC-midnight instant to its date', () => {
    expect(calendarDateOnly('2026-09-21T00:00:00Z')).toBe('2026-09-21');
    expect(calendarDateOnly('2026-09-21T00:00:00.000Z')).toBe('2026-09-21');
    expect(calendarDateOnly('2026-09-21T00:00:00+00:00')).toBe('2026-09-21');
  });

  test('leaves date-only values and real instants untouched', () => {
    expect(calendarDateOnly('2026-09-21')).toBe('2026-09-21');
    expect(calendarDateOnly('2026-09-21T14:00:00Z')).toBe('2026-09-21T14:00:00Z');
    expect(calendarDateOnly('2026-09-21T00:00:00-05:00')).toBe('2026-09-21T00:00:00-05:00');
    expect(calendarDateOnly('')).toBe('');
  });

  test('a T00:00:00Z calendar value renders the API date in America/Chicago', () => {
    useTimeZone('America/Chicago');
    // Documents the bug: rendered as an instant, the 21st shows as the 20th.
    expect(formatDateWithFormat('2026-09-21T00:00:00Z', 'iso')).toBe('2026-09-20');
    // The calendar-field path shows the API's own date.
    expect(formatDateWithFormat(calendarDateOnly('2026-09-21T00:00:00Z'), 'iso')).toBe(
      '2026-09-21',
    );
    // And the new date-only wire value is unaffected by the zone.
    expect(formatDateWithFormat(calendarDateOnly('2026-09-21'), 'eu')).toBe('21.09.2026');
  });
});

describe('isBeforeLocalToday (#1778)', () => {
  test('compares calendar days west of UTC, not instants', () => {
    useTimeZone('America/Chicago');
    // 10:00 local on 11 March; today's 00:00Z is the previous evening locally.
    const now = new Date('2026-03-11T15:00:00Z');
    expect(isBeforeLocalToday('2026-03-11T00:00:00Z', now)).toBe(false);
    expect(isBeforeLocalToday('2026-03-11', now)).toBe(false);
    expect(isBeforeLocalToday('2026-03-10T00:00:00Z', now)).toBe(true);
    expect(isBeforeLocalToday('2026-03-12', now)).toBe(false);
  });

  test('a real instant is compared against local midnight', () => {
    useTimeZone('America/Chicago');
    const now = new Date('2026-03-11T15:00:00Z');
    // 04:00Z on 11 March is 23:00 on 10 March locally: before today.
    expect(isBeforeLocalToday('2026-03-11T04:00:00Z', now)).toBe(true);
    // 06:00Z on 11 March is 01:00 on 11 March locally: today.
    expect(isBeforeLocalToday('2026-03-11T06:00:00Z', now)).toBe(false);
  });
});

describe('localDaysUntil (issue #2679)', () => {
  // The server counts days from the operator's REMINDER_TIMEZONE (UTC here);
  // the screen must count from the viewer's own day, like every other date.
  // 01:30Z on 17 Nov is still the evening of 16 Nov in Chicago.
  const serverNextDay = new Date('2026-11-17T01:30:00Z');

  test('a birthday today in the browser zone is "Today", though the server is a day ahead', () => {
    useTimeZone('America/Chicago');
    expect(localDaysUntil('2026-11-16', serverNextDay)).toBe(0);
    expect(localDaysUntil('2026-11-17', serverNextDay)).toBe(1);
    expect(localDaysUntil('2026-12-16', serverNextDay)).toBe(30);
  });

  test('is unaffected by the time of day', () => {
    useTimeZone('America/Chicago');
    expect(localDaysUntil('2026-11-17', new Date('2026-11-16T06:01:00Z'))).toBe(1);
    expect(localDaysUntil('2026-11-17', new Date('2026-11-17T05:59:00Z'))).toBe(1);
  });

  test('east of UTC the local day can be ahead of the server day', () => {
    useTimeZone('Pacific/Auckland');
    // 08:00 on 12 June in Auckland == 20:00Z on 11 June.
    expect(localDaysUntil('2026-06-12', new Date('2026-06-11T20:00:00Z'))).toBe(0);
  });

  test('a date already past on this device clamps to Today', () => {
    useTimeZone('America/Chicago');
    expect(localDaysUntil('2026-11-10', serverNextDay)).toBe(0);
  });

  test('crosses a daylight-saving change without drifting by a day', () => {
    useTimeZone('America/Chicago'); // spring forward on 2026-03-08: a 23-hour day
    expect(localDaysUntil('2026-03-09', new Date('2026-03-07T18:00:00Z'))).toBe(2);
    expect(localDaysUntil('2026-03-09', new Date('2026-03-08T23:30:00Z'))).toBe(1);
  });

  test('accepts a midnight-UTC instant as the calendar day it names', () => {
    useTimeZone('America/Chicago');
    expect(localDaysUntil('2026-11-17T00:00:00Z', serverNextDay)).toBe(1);
  });

  test('a value that is not a calendar date counts as Today rather than NaN', () => {
    useTimeZone('America/Chicago');
    expect(localDaysUntil('', serverNextDay)).toBe(0);
    expect(localDaysUntil('soon', serverNextDay)).toBe(0);
  });
});
