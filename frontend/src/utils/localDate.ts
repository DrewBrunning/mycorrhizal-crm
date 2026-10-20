/**
 * The user's *local* calendar date as `YYYY-MM-DD`, for `<input type="date">`
 * values and `min` attributes.
 *
 * Do NOT use `new Date().toISOString().split('T')[0]` for "today": that is the
 * UTC date, so west of UTC in the evening (20:00 in New York is 00:00 UTC the
 * next day) a new activity/note/reminder defaults to *tomorrow*, and east of
 * UTC early in the morning it defaults to yesterday (issue #1727).
 *
 * The edit paths (NetworkPage, NotesPage, ActivitiesPage, useTimelineEditing)
 * deliberately keep `toISOString().split('T')[0]` — but they convert a
 * *stored* value, which is a date saved at 00:00Z, so its UTC calendar date is
 * the date the user picked. "Now" is an instant in the user's own day, so it
 * must be read in the local zone.
 */
export function localISODate(date: Date): string {
  const year = String(date.getFullYear()).padStart(4, '0');
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

/** Today's local calendar date as `YYYY-MM-DD`. `now` is injectable for tests. */
export function todayLocalISODate(now: Date = new Date()): string {
  return localISODate(now);
}

// The suffixes (after the 10-char date) of an RFC 3339 instant at exactly UTC
// midnight: `Z`, fractional zeros, or an explicit zero offset. A literal list,
// not a regex, so there is nothing for the unsafe-regex lint to object to.
const UTC_MIDNIGHT_SUFFIXES = new Set([
  'T00:00:00Z',
  'T00:00:00.000Z',
  'T00:00:00+00:00',
  'T00:00:00.000+00:00',
]);
const DATE_PREFIX = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Normalizes a *calendar-only* field (cadence / data-decay `next_due`) to a
 * date-only `YYYY-MM-DD` so it renders on the same day in every time zone.
 *
 * The API now emits these fields date-only (issue #1778), which passes through
 * unchanged. This is the fallback for a server that still sends the old
 * midnight-UTC instant: `formatDate` renders a timestamp in the *local* zone
 * (correct for real instants, #962), so `2026-09-21T00:00:00Z` shows as the
 * 20th west of UTC. Only an exact UTC midnight is rewritten; any other
 * instant is returned untouched.
 *
 * Use it ONLY on fields documented as calendar days, never on real
 * timestamps such as `remind_at` or an activity's `date`.
 */
export function calendarDateOnly(value: string): string {
  const date = value.slice(0, 10);
  return DATE_PREFIX.test(date) && UTC_MIDNIGHT_SUFFIXES.has(value.slice(10)) ? date : value;
}
