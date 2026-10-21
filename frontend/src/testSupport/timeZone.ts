import { vi } from 'vitest';

// Simulates a process time zone for tests by making Date's local getters (and
// setHours) answer for `timeZone`, computed with Intl.
//
// Why not mutate the TZ environment variable? That works in a normal process
// but NOT in a worker thread: V8 reads the zone once when the isolate starts
// and ignores later env changes. Stryker's vitest runner hard-codes `pool: 'threads'`, so
// every timezone test that changed TZ at runtime failed its dry run
// (issue #1843) while passing under the project's forks pool. This keeps the
// assertions real -- the code still reads Date's local getters, so a return to
// UTC getters (the #1727/#962 bugs) still fails -- without relying on runtime
// env mutation.
//
// Call from a test (or beforeEach/beforeAll); restore with vi.restoreAllMocks()
// or the returned function.

interface ZonedParts {
  year: number;
  month: number; // zero-based, like Date#getMonth
  day: number;
  hour: number;
  minute: number;
  second: number;
}

const formatters = new Map<string, Intl.DateTimeFormat>();

function formatter(timeZone: string): Intl.DateTimeFormat {
  let cached = formatters.get(timeZone);
  if (!cached) {
    cached = new Intl.DateTimeFormat('en-US', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    });
    formatters.set(timeZone, cached);
  }
  return cached;
}

function zonedParts(date: Date, timeZone: string): ZonedParts {
  const parts = formatter(timeZone).formatToParts(date);
  const value = (type: Intl.DateTimeFormatPartTypes): number => {
    const part = parts.find((p) => p.type === type);
    return part ? Number(part.value) : 0;
  };
  return {
    year: value('year'),
    month: value('month') - 1,
    day: value('day'),
    hour: value('hour'),
    minute: value('minute'),
    second: value('second'),
  };
}

// Date#getTimezoneOffset semantics: minutes to add to local time to reach UTC
// (positive west of UTC, negative east).
function zonedOffsetMinutes(date: Date, timeZone: string): number {
  const p = zonedParts(date, timeZone);
  const asUTC = Date.UTC(p.year, p.month, p.day, p.hour, p.minute, p.second);
  return Math.round((date.getTime() - asUTC) / 60_000);
}

/**
 * Make Date's local-time getters and setters behave as if the process were in
 * `timeZone`. Returns a restore function; calling `vi.restoreAllMocks()` also
 * works.
 */
export function useTimeZone(timeZone: string): () => void {
  const proto = Date.prototype;
  const spies = [
    vi.spyOn(proto, 'getFullYear').mockImplementation(function (this: Date) {
      return zonedParts(this, timeZone).year;
    }),
    vi.spyOn(proto, 'getMonth').mockImplementation(function (this: Date) {
      return zonedParts(this, timeZone).month;
    }),
    vi.spyOn(proto, 'getDate').mockImplementation(function (this: Date) {
      return zonedParts(this, timeZone).day;
    }),
    vi.spyOn(proto, 'getDay').mockImplementation(function (this: Date) {
      const p = zonedParts(this, timeZone);
      return new Date(Date.UTC(p.year, p.month, p.day)).getUTCDay();
    }),
    vi.spyOn(proto, 'getHours').mockImplementation(function (this: Date) {
      return zonedParts(this, timeZone).hour;
    }),
    vi.spyOn(proto, 'getMinutes').mockImplementation(function (this: Date) {
      return zonedParts(this, timeZone).minute;
    }),
    vi.spyOn(proto, 'getSeconds').mockImplementation(function (this: Date) {
      return zonedParts(this, timeZone).second;
    }),
    vi.spyOn(proto, 'getTimezoneOffset').mockImplementation(function (this: Date) {
      return zonedOffsetMinutes(this, timeZone);
    }),
    vi.spyOn(proto, 'setHours').mockImplementation(function (
      this: Date,
      hours: number,
      minutes?: number,
      seconds?: number,
      ms?: number,
    ) {
      const p = zonedParts(this, timeZone);
      const offset = zonedOffsetMinutes(this, timeZone);
      const utc = Date.UTC(
        p.year,
        p.month,
        p.day,
        hours,
        minutes ?? p.minute,
        seconds ?? p.second,
        ms ?? this.getMilliseconds(),
      );
      this.setTime(utc + offset * 60_000);
      return this.getTime();
    }),
  ];
  return () => {
    for (const spy of spies.reverse()) spy.mockRestore();
  };
}
