// Link periods (ADR 0025, ADR 0043 decision 2). The web UI edits whole years,
// like the work/address periods; a finer date already stored is kept when its
// year is not touched.
import type { TemporalRange } from '../api/entities';
import type { PartialDate } from '../api/lifeEvents';

type Day = { year: number; month: number; day: number };

// -1/0/1 comparing a to b at the precision both carry; 0 when they cannot be
// compared (ADR 0015 rule 3: non-comparability is not an error, it counts as
// current).
function comparePartial(a: PartialDate, b: Day): number {
  if (a.year == null) return 0;
  if (a.year !== b.year) return a.year < b.year ? -1 : 1;
  if (a.month == null) return 0;
  if (a.month !== b.month) return a.month < b.month ? -1 : 1;
  if (a.day == null) return 0;
  if (a.day !== b.day) return a.day < b.day ? -1 : 1;
  return 0;
}

/** True when `today` is inside the link's period (mirrors the backend's periodIncludes). */
export function isLinkCurrent(period: TemporalRange | undefined | null, today: Day): boolean {
  if (!period) return true;
  if (period.start && comparePartial(period.start, today) > 0) return false;
  if (period.end && comparePartial(period.end, today) < 0) return false;
  return true;
}

export function yearsFromPeriod(period: TemporalRange | undefined | null) {
  return {
    startYear: period?.start?.year != null ? String(period.start.year) : '',
    endYear: period?.end?.year != null ? String(period.end.year) : '',
  };
}

const YEAR = /^\d{4}$/;

/** Whole-year text -> period; blank both -> undefined; bad text or end < start -> 'invalid'. */
export function periodFromYears(
  startText: string,
  endText: string,
  existing?: TemporalRange | null,
): TemporalRange | undefined | 'invalid' {
  const s = startText.trim();
  const e = endText.trim();
  if ((s && !YEAR.test(s)) || (e && !YEAR.test(e))) return 'invalid';
  if (s && e && Number(e) < Number(s)) return 'invalid';
  const keep = (text: string, prior?: PartialDate): PartialDate | undefined =>
    !text ? undefined : prior?.year === Number(text) ? prior : { year: Number(text) };
  const start = keep(s, existing?.start);
  const end = keep(e, existing?.end);
  if (!start && !end) return undefined;
  return { ...(start ? { start } : {}), ...(end ? { end } : {}) };
}

type TFn = (key: string, options?: Record<string, unknown>) => string;

/** "2019 – 2021", "Since 2019", "Until 2021", or '' for no period. */
export function formatLinkPeriod(period: TemporalRange | undefined | null, t: TFn): string {
  const { startYear, endYear } = yearsFromPeriod(period);
  if (startYear && endYear) return `${startYear} – ${endYear}`;
  if (startYear) return t('contactDetail.period.since', { year: startYear });
  if (endYear) return t('contactDetail.period.until', { year: endYear });
  return '';
}

/** Today's calendar day in the browser's zone, for the current-link check. */
export function localToday(now: Date = new Date()): Day {
  return { year: now.getFullYear(), month: now.getMonth() + 1, day: now.getDate() };
}
