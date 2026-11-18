// Issue #2762 / #2679: display endpoints that decide "today" (dashboard,
// upcoming occasions, gift shopping list, birthdays, upcoming reminders)
// accept an optional `tz` IANA zone name so "today" follows the viewer's
// browser instead of the server's REMINDER_TIMEZONE (ADR 0015 Rule 4).
// The server rejects an empty-after-trim, over-long (> 64 chars), unknown or
// `Local` zone with 400 validation_timezone_invalid, so an unusable browser
// zone is omitted here and the server default applies silently.

// Mirrors the backend's maxLength for the `tz` parameter (openapi ViewerTimezone).
const MAX_TZ_LENGTH = 64;

/** The browser's IANA zone, or undefined when it is missing or unusable. */
export function viewerTimezone(): string | undefined {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (typeof zone !== 'string') return undefined;
    const trimmed = zone.trim();
    if (trimmed === '' || trimmed === 'Local' || trimmed.length > MAX_TZ_LENGTH) return undefined;
    return trimmed;
  } catch {
    return undefined;
  }
}

/** Appends `tz=<browser zone>` to a URL when a usable zone exists. */
export function withViewerTimezone(url: string): string {
  const zone = viewerTimezone();
  if (!zone) return url;
  return `${url}${url.includes('?') ? '&' : '?'}tz=${encodeURIComponent(zone)}`;
}
