import { ApiError } from '../api/client';
import { type DateFormat, formatDateWithFormat } from '../DateFormatProvider';
import { calendarDateOnly } from './localDate';

// Issue #2006: an activity may carry an optional inclusive `end_date`, a
// calendar day on the wire exactly like `date` (an RFC 3339 instant at
// 00:00:00Z, read back as its UTC date; ADR 0015). These helpers keep the
// day-picker <-> wire conversion and the range display in one place.

/** The `YYYY-MM-DD` a stored calendar-day instant denotes (its UTC date), or ''. */
export function activityDay(value: string | null | undefined): string {
  if (!value) return '';
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? '' : time.toISOString().split('T')[0];
}

/** The wire instant for a picked `YYYY-MM-DD` day (00:00:00Z). */
export function activityDayToWire(day: string): string {
  return new Date(day).toISOString();
}

/**
 * The `end_date` to send on a PUT: the picked day, or `null` to clear it.
 * Always sent from an editor, because an omitted key means "keep the stored
 * value" and an editor that blanked the field must clear it.
 */
export function activityEndForUpdate(end: string | undefined): string | null {
  return end ? activityDayToWire(end) : null;
}

/**
 * The wire `{date, end_date?}` for a dialog's picked days (`YYYY-MM-DD`); spread
 * it over the dialog payload. `end_date` is present only when an end was picked.
 */
export function activityWireDates(activity: { date: string; end_date?: string }): {
  date: string;
  end_date?: string;
} {
  return {
    date: activityDayToWire(activity.date),
    ...(activity.end_date ? { end_date: activityDayToWire(activity.end_date) } : {}),
  };
}

/** The server's message when a save was refused on the `end_date` field, else undefined. */
export function endDateFieldError(err: unknown): string | undefined {
  if (err instanceof ApiError && err.details?.field === 'end_date') {
    return err.getDisplayMessage();
  }
  return undefined;
}

const MONTH_NAME_FORMATS: DateFormat[] = ['us-mmm', 'us-mmmm', 'eu-mmm', 'eu-mmmm'];

/**
 * An activity's date as the user's date-format preference renders it: one day,
 * or "start – end" for a range. In the month-name formats a range inside one
 * month collapses ("12–15 May, 2026" / "May 12–15, 2026"); the numeric formats
 * always show both ends in full, since collapsing "12.05.2026" is ambiguous.
 * A missing, equal or reversed end is shown as the single start day.
 */
export function formatActivityDateRange(
  start: string,
  end: string | null | undefined,
  format: DateFormat,
): string {
  const startDay = calendarDateOnly(start);
  const endDay = end ? calendarDateOnly(end) : '';
  const startIso = activityDay(startDay);
  const endIso = activityDay(endDay);
  if (!startIso || !endIso || endIso <= startIso) return formatDateWithFormat(startDay, format);

  const [sy, sm, sd] = startIso.split('-');
  const [ey, em, ed] = endIso.split('-');
  if (sy === ey && sm === em && MONTH_NAME_FORMATS.includes(format)) {
    const yearless = (mm: string, dd: string) => formatDateWithFormat(`--${mm}-${dd}`, format);
    switch (format) {
      case 'eu-mmm':
        return `${Number(sd)}–${yearless(em, ed)}, ${ey}`;
      case 'eu-mmmm':
        return `${sd}–${yearless(em, ed)}, ${ey}`;
      case 'us-mmm':
      case 'us-mmmm':
        return `${yearless(sm, sd)}–${Number(ed)}, ${ey}`;
    }
  }
  return `${formatDateWithFormat(startDay, format)} – ${formatDateWithFormat(endDay, format)}`;
}
