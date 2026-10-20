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
 * Normalizes a calendar day that travels as a midnight-UTC instant to a
 * date-only `YYYY-MM-DD`, so it renders on the same day in every time zone.
 *
 * `formatDate` renders a timestamp in the *local* zone (correct for real
 * instants, #962), so `2026-09-21T00:00:00Z` shows as the 20th west of UTC.
 * Only an exact UTC midnight is rewritten; any other instant is returned
 * untouched, so a real timestamp still renders in the local zone.
 *
 * Use it on the fields whose value is a picked or derived calendar day:
 * - cadence / data-decay `next_due`: the API now emits these date-only
 *   (issue #1778), which passes through; this is the fallback for an older
 *   server's midnight-UTC instant.
 * - a reminder's `remind_at`: day-granular, stored at 00:00Z (ADR 0015).
 * - an activity's / note's `date` and cadence `last_interaction` (an activity
 *   date): the date input saves `new Date('YYYY-MM-DD')`, i.e. 00:00Z, and the
 *   edit paths read it back as its UTC date (#1727). Rendering it in the local
 *   zone showed an activity logged "today" as yesterday west of UTC, and a
 *   different day in the list than in its own edit form.
 *
 * Not for values written at local midnight (gifts' `date`, which GiftDialog
 * stores as local midnight and reads back in the local zone) or for real
 * instants (`*_at`, `discussed_at`, external activities' `occurred_at`).
 */
export function calendarDateOnly(value: string): string {
  const date = value.slice(0, 10);
  return DATE_PREFIX.test(date) && UTC_MIDNIGHT_SUFFIXES.has(value.slice(10)) ? date : value;
}

/**
 * True when a calendar-day value (see {@link calendarDateOnly}) falls before
 * the user's local today. Compares calendar digits, so a reminder due today
 * and stored at 00:00Z is not "overdue" west of UTC, where that instant is the
 * previous evening. A real instant is compared against local midnight.
 */
export function isBeforeLocalToday(value: string, now: Date = new Date()): boolean {
  const day = calendarDateOnly(value);
  if (DATE_PREFIX.test(day)) return day < todayLocalISODate(now);
  const midnight = new Date(now);
  midnight.setHours(0, 0, 0, 0);
  return new Date(value) < midnight;
}
