import { todayLocalISODate } from './localDate';

/**
 * The age a person has on, or will turn at, the next occurrence of a birthday
 * (issue #2681). `month` and `day` are the two-digit parts of the stored
 * `YYYY-MM-DD` birthday. `isToday` is true when that occurrence is today, i.e.
 * they already are that age; otherwise they are still to turn it. A birthday
 * whose date has passed this year rolls to next year. Compares calendar
 * digits in the viewer's local zone.
 */
export function upcomingBirthdayAge(
  birthYear: number,
  month: string,
  day: string,
  now: Date = new Date(),
): { age: number; isToday: boolean } {
  const today = todayLocalISODate(now);
  const year = now.getFullYear();
  const thisYear = `${String(year).padStart(4, '0')}-${month}-${day}`;
  if (thisYear === today) return { age: year - birthYear, isToday: true };
  return { age: (thisYear > today ? year : year + 1) - birthYear, isToday: false };
}
