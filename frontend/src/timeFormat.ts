import { useCallback, useSyncExternalStore } from 'react';
import {
  type DateFormat,
  formatDateWithFormat,
  getDateFormat,
  subscribeDateFormat,
} from './DateFormatProvider';

// The user's 12/24-hour preference (issue #1830). Persisted backend-side as
// users.time_format, a stable contract shared with backend/models/user.go
// (TimeFormats / the `oneof` validator), backend/controllers/user_controller.go
// (UpdateTimeFormat) and backend/openapi.yaml — if you add a value here, add it
// in all of them.
export type TimeFormat = 'auto' | '12h' | '24h';

// Frontend enum mirror of the backend `oneof=auto 12h 24h` validator — keep in
// sync by hand (see /CLAUDE.md frontend trap #4).
export const TIME_FORMATS: readonly TimeFormat[] = ['auto', '12h', '24h'];

export function isTimeFormat(value: unknown): value is TimeFormat {
  return typeof value === 'string' && (TIME_FORMATS as readonly string[]).includes(value);
}

/**
 * Maps a time-format preference to the Intl options that realise it.
 *
 * Browsers do not expose the OS 12/24-hour setting, so `auto` means "the
 * locale's own default": `hourCycle` is left unset. `12h`/`24h` override it.
 * `h23` (not `hour12: false`) so midnight renders as 00:xx, never 24:xx.
 */
export function timeFormatOptions(timeFormat: TimeFormat): Intl.DateTimeFormatOptions {
  switch (timeFormat) {
    case '12h':
      return { hourCycle: 'h12' };
    case '24h':
      return { hourCycle: 'h23' };
    default:
      return {};
  }
}

// What `Date.prototype.toLocaleString()` renders when given no field options.
const DATE_TIME_FIELDS: Intl.DateTimeFormatOptions = {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: 'numeric',
  minute: 'numeric',
  second: 'numeric',
};

const TIME_FIELDS: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: 'numeric' };

// A timestamp has no seconds: they add noise to an audit/event list and the
// user cannot act on them. The date half follows the user's Date format
// preference; the time half follows their 12/24-hour preference.
const PREFERRED_TIME_FIELDS: Intl.DateTimeFormatOptions = {
  hour: 'numeric',
  minute: '2-digit',
};

function toDate(value: string | number | Date): Date | null {
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

/**
 * Date + time, like `toLocaleString()`, with the hour cycle per `timeFormat`.
 * `options` replaces the default field set (it is not merged into it), so a
 * caller can ask for e.g. a `dateStyle`/`timeStyle` pair. An unparseable
 * value is returned as-is (as a string) rather than throwing.
 */
export function formatDateTimeWith(
  value: string | number | Date,
  timeFormat: TimeFormat,
  locale?: string | string[],
  options?: Intl.DateTimeFormatOptions,
): string {
  const date = toDate(value);
  if (!date) return String(value);
  return date.toLocaleString(locale, {
    ...(options ?? DATE_TIME_FIELDS),
    ...timeFormatOptions(timeFormat),
  });
}

/**
 * Time of day only (hour + minute by default), with the hour cycle per
 * `timeFormat`. An unparseable value is returned as-is.
 */
export function formatTimeWith(
  value: string | number | Date,
  timeFormat: TimeFormat,
  locale?: string | string[],
  options?: Intl.DateTimeFormatOptions,
): string {
  const date = toDate(value);
  if (!date) return String(value);
  return date.toLocaleTimeString(locale, {
    ...(options ?? TIME_FIELDS),
    ...timeFormatOptions(timeFormat),
  });
}

/**
 * A timestamp in the user's preferred date format plus a locale time with no
 * seconds (e.g. `26.10.2026 13:06` or `10/26/2026 1:06 PM`). The instant is
 * rendered in the local zone — these are real instants, not calendar days
 * (see #962/#1778 for the distinction). An unparseable value is returned
 * as-is.
 */
export function formatPreferredDateTime(
  value: string | number | Date,
  dateFormat: DateFormat,
  timeFormat: TimeFormat,
  locale?: string | string[],
): string {
  const date = toDate(value);
  if (!date) return String(value);
  const datePart = formatDateWithFormat(date.toISOString(), dateFormat);
  const timePart = formatTimeWith(date, timeFormat, locale, PREFERRED_TIME_FIELDS);
  return `${datePart} ${timePart}`;
}

// --- the current preference ---------------------------------------------------
//
// Held in a tiny module-level store (backed by localStorage, the same way the
// date format survives a reload) rather than React context, because most of the
// time renders live in plain module-level helpers that are not components.
// `useTimeFormat()` subscribes, so a component that shows the setting re-renders
// when it changes.

const TIME_FORMAT_STORAGE_KEY = 'timeFormat';

function readStored(): TimeFormat {
  try {
    const stored =
      typeof window === 'undefined' ? null : window.localStorage.getItem(TIME_FORMAT_STORAGE_KEY);
    return isTimeFormat(stored) ? stored : 'auto';
  } catch {
    return 'auto';
  }
}

let current: TimeFormat = readStored();
const listeners = new Set<() => void>();

export function getTimeFormat(): TimeFormat {
  return current;
}

export function setTimeFormat(timeFormat: TimeFormat): void {
  if (!isTimeFormat(timeFormat)) return;
  try {
    window.localStorage.setItem(TIME_FORMAT_STORAGE_KEY, timeFormat);
  } catch {
    // Storage unavailable (private mode): the in-memory value still applies.
  }
  if (timeFormat === current) return;
  current = timeFormat;
  for (const listener of listeners) listener();
}

// Initialise from the backend value on login, like initializeDateFormatFromBackend.
export function initializeTimeFormatFromBackend(timeFormat: string | undefined): void {
  if (isTimeFormat(timeFormat)) setTimeFormat(timeFormat);
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * A date rendered in the user's Date format preference (the local calendar
 * date of a timestamp). Use this instead of `toLocaleDateString()` for a
 * displayed calendar day — the browser locale ignores the app preference.
 */
export function formatLocalDate(
  value: string | number | Date,
  dateFormat: DateFormat = getDateFormat(),
): string {
  const date = toDate(value);
  if (!date) return String(value);
  return formatDateWithFormat(date.toISOString(), dateFormat);
}

/**
 * The current preference, as a module-level read (use outside components).
 * With no explicit field options this is the preferred date format + `HH:mm`
 * (no seconds); pass `options` to override the field set entirely.
 */
export function formatDateTime(
  value: string | number | Date,
  options?: Intl.DateTimeFormatOptions,
): string {
  if (options) return formatDateTimeWith(value, current, undefined, options);
  return formatPreferredDateTime(value, getDateFormat(), current);
}

/** Time of day in the current preference (use outside components). */
export function formatTime(
  value: string | number | Date,
  options?: Intl.DateTimeFormatOptions,
): string {
  return formatTimeWith(value, current, undefined, options);
}

/** React binding: re-renders when the preference changes. */
export function useTimeFormat(): {
  timeFormat: TimeFormat;
  setTimeFormat: (timeFormat: TimeFormat) => void;
  formatDateTime: typeof formatDateTime;
  formatTime: typeof formatTime;
} {
  const timeFormat = useSyncExternalStore(subscribe, getTimeFormat, getTimeFormat);
  const dateFormat = useSyncExternalStore(subscribeDateFormat, getDateFormat, getDateFormat);
  const dateTime = useCallback(
    (value: string | number | Date, options?: Intl.DateTimeFormatOptions) =>
      options
        ? formatDateTimeWith(value, timeFormat, undefined, options)
        : formatPreferredDateTime(value, dateFormat, timeFormat),
    [dateFormat, timeFormat],
  );
  const time = useCallback(
    (value: string | number | Date, options?: Intl.DateTimeFormatOptions) =>
      formatTimeWith(value, timeFormat, undefined, options),
    [timeFormat],
  );
  return { timeFormat, setTimeFormat, formatDateTime: dateTime, formatTime: time };
}

/**
 * The timestamp formatter bound to both the date-format and 12/24-hour
 * preferences, re-rendering the calling component when either changes. Use it
 * instead of the module-level `formatDateTime` in components.
 */
export function useFormatDateTime(): (value: string | number | Date) => string {
  const dateFormat = useSyncExternalStore(subscribeDateFormat, getDateFormat, getDateFormat);
  const timeFormat = useSyncExternalStore(subscribe, getTimeFormat, getTimeFormat);
  return useCallback(
    (value: string | number | Date) => formatPreferredDateTime(value, dateFormat, timeFormat),
    [dateFormat, timeFormat],
  );
}
