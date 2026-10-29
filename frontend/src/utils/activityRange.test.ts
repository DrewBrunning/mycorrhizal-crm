import { describe, expect, test } from 'vitest';
import { ApiError } from '../api/client';
import '../i18n/config';
import {
  activityDay,
  activityDayToWire,
  activityEndForUpdate,
  activityWireDates,
  endDateFieldError,
  formatActivityDateRange,
} from './activityRange';

describe('activityDay / wire conversion', () => {
  test('reads the UTC calendar day of a stored instant, or empty', () => {
    expect(activityDay('2026-05-15T00:00:00Z')).toBe('2026-05-15');
    expect(activityDay(undefined)).toBe('');
    expect(activityDay(null)).toBe('');
    expect(activityDay('not a date')).toBe('');
  });

  test('a picked day goes out as 00:00:00Z', () => {
    expect(activityDayToWire('2026-05-15')).toBe('2026-05-15T00:00:00.000Z');
  });

  test('an editor clears the end with null and sets it with the instant', () => {
    expect(activityEndForUpdate('')).toBeNull();
    expect(activityEndForUpdate(undefined)).toBeNull();
    expect(activityEndForUpdate('2026-05-15')).toBe('2026-05-15T00:00:00.000Z');
  });

  test('create dates omit end_date unless one was picked', () => {
    expect(activityWireDates({ date: '2026-05-12' })).toEqual({ date: '2026-05-12T00:00:00.000Z' });
    expect(activityWireDates({ date: '2026-05-12', end_date: '' })).toEqual({
      date: '2026-05-12T00:00:00.000Z',
    });
    expect(activityWireDates({ date: '2026-05-12', end_date: '2026-05-15' })).toEqual({
      date: '2026-05-12T00:00:00.000Z',
      end_date: '2026-05-15T00:00:00.000Z',
    });
  });
});

describe('formatActivityDateRange', () => {
  const start = '2026-05-12T00:00:00Z';
  const end = '2026-05-15T00:00:00Z';

  test('a single day, or an equal / reversed / unparseable end, shows the start only', () => {
    expect(formatActivityDateRange(start, undefined, 'eu')).toBe('12.05.2026');
    expect(formatActivityDateRange(start, null, 'eu')).toBe('12.05.2026');
    expect(formatActivityDateRange(start, start, 'eu')).toBe('12.05.2026');
    expect(formatActivityDateRange(start, '2026-05-01T00:00:00Z', 'eu')).toBe('12.05.2026');
    expect(formatActivityDateRange(start, 'garbage', 'eu')).toBe('12.05.2026');
  });

  test('numeric formats show both ends in full, in the preferred order', () => {
    expect(formatActivityDateRange(start, end, 'eu')).toBe('12.05.2026 – 15.05.2026');
    expect(formatActivityDateRange(start, end, 'us')).toBe('05/12/2026 – 05/15/2026');
    expect(formatActivityDateRange(start, end, 'iso')).toBe('2026-05-12 – 2026-05-15');
    expect(formatActivityDateRange(start, end, 'ca')).toBe('12/05/2026 – 15/05/2026');
    expect(formatActivityDateRange(start, end, 'eu-hyphen')).toBe('12-05-2026 – 15-05-2026');
  });

  test('month-name formats collapse a range inside one month', () => {
    expect(formatActivityDateRange(start, end, 'eu-mmm')).toBe('12–15 May, 2026');
    expect(formatActivityDateRange(start, end, 'eu-mmmm')).toBe('12–15 May, 2026');
    expect(formatActivityDateRange('2026-05-02T00:00:00Z', end, 'eu-mmmm')).toBe('02–15 May, 2026');
    expect(formatActivityDateRange(start, end, 'us-mmm')).toBe('May 12–15, 2026');
    expect(formatActivityDateRange(start, end, 'us-mmmm')).toBe('May 12–15, 2026');
  });

  test('month-name formats show both ends when the range crosses a month or year', () => {
    expect(formatActivityDateRange(start, '2026-06-02T00:00:00Z', 'eu-mmm')).toBe(
      '12 May, 2026 – 2 Jun, 2026',
    );
    expect(formatActivityDateRange('2026-12-30T00:00:00Z', '2027-01-02T00:00:00Z', 'us-mmmm')).toBe(
      'December 30, 2026 – January 2, 2027',
    );
  });

  test('is a calendar day: no shift west of UTC for a midnight-UTC instant', () => {
    // Pure string/UTC arithmetic: the same output whatever zone the runner uses.
    expect(formatActivityDateRange('2026-01-01T00:00:00Z', '2026-01-03T00:00:00Z', 'iso')).toBe(
      '2026-01-01 – 2026-01-03',
    );
  });
});

describe('endDateFieldError', () => {
  test('returns the translated message only for a refusal on end_date', () => {
    const onEnd = new ApiError(
      'Invalid value for field end_date',
      'INVALID_INPUT',
      400,
      { field: 'end_date', reason: 'end date precedes the start date' },
      undefined,
      'activity_end_before_start',
    );
    expect(endDateFieldError(onEnd)).toBe('The end date cannot be before the start date');

    const elsewhere = new ApiError('bad', 'INVALID_INPUT', 400, { field: 'title' });
    expect(endDateFieldError(elsewhere)).toBeUndefined();
    expect(endDateFieldError(new Error('x'))).toBeUndefined();
    expect(endDateFieldError(new ApiError('bad', 'X', 500))).toBeUndefined();
  });
});
