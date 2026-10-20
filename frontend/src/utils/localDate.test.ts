import { afterEach, describe, expect, test, vi } from 'vitest';
import { formatDateWithFormat } from '../DateFormatProvider';
import { calendarDateOnly, localISODate, todayLocalISODate } from './localDate';

// Changing process.env.TZ at runtime moves Date's local getters in this
// Node/V8 build (same technique as DateFormatProvider.test.ts), so these are
// real timezone regressions, not simulated ones.
const originalTZ = process.env.TZ;

afterEach(() => {
  process.env.TZ = originalTZ;
  vi.useRealTimers();
});

describe('todayLocalISODate (#1727)', () => {
  test('west of UTC in the evening, today is the local date, not the UTC date', () => {
    process.env.TZ = 'America/Chicago'; // CDT, UTC-05:00 on this date
    // 22:30 local on 11 March == 03:30Z on 12 March.
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-03-12T03:30:00Z'));
    expect(todayLocalISODate()).toBe('2026-03-11');
  });

  test('east of UTC in the morning, today is the local date, not the UTC date', () => {
    process.env.TZ = 'Pacific/Auckland'; // NZST, UTC+12 in June
    // 08:00 local on 12 June == 20:00Z on 11 June.
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-06-11T20:00:00Z'));
    expect(todayLocalISODate()).toBe('2026-06-12');
  });

  test('accepts an injected clock and zero-pads', () => {
    process.env.TZ = 'UTC';
    expect(todayLocalISODate(new Date('2026-01-02T12:00:00Z'))).toBe('2026-01-02');
    expect(localISODate(new Date(2026, 0, 5))).toBe('2026-01-05');
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
    process.env.TZ = 'America/Chicago';
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
