import { useCallback, useSyncExternalStore } from 'react';

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

/** The current preference, as a module-level read (use outside components). */
export function formatDateTime(
  value: string | number | Date,
  options?: Intl.DateTimeFormatOptions,
): string {
  return formatDateTimeWith(value, current, undefined, options);
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
  const dateTime = useCallback(
    (value: string | number | Date, options?: Intl.DateTimeFormatOptions) =>
      formatDateTimeWith(value, timeFormat, undefined, options),
    [timeFormat],
  );
  const time = useCallback(
    (value: string | number | Date, options?: Intl.DateTimeFormatOptions) =>
      formatTimeWith(value, timeFormat, undefined, options),
    [timeFormat],
  );
  return { timeFormat, setTimeFormat, formatDateTime: dateTime, formatTime: time };
}
