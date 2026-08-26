import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import {
  autoFormatBirthdayInputWithFormat,
  calculateAgeFromBirthday,
  formatBirthdayForInputWithFormat,
  formatBirthdayWithFormat,
  formatDateWithFormat,
  parseBirthdayInputWithFormat,
} from './DateFormatProvider';

describe('formatDateWithFormat', () => {
  test('formats ISO dates in EU and US styles', () => {
    expect(formatDateWithFormat('2026-06-11', 'eu')).toBe('11.06.2026');
    expect(formatDateWithFormat('2026-06-11', 'us')).toBe('06/11/2026');
    expect(formatDateWithFormat('2026-06-11', 'iso')).toBe('2026-06-11');
  });

  test('formats in the built-out numeric formats', () => {
    expect(formatDateWithFormat('2026-06-11', 'ca')).toBe('11/06/2026');
    expect(formatDateWithFormat('2026-06-11', 'eu-hyphen')).toBe('11-06-2026');
  });

  test('formats in the month-name formats', () => {
    expect(formatDateWithFormat('2026-06-11', 'us-mmm')).toBe('Jun 11, 2026');
    expect(formatDateWithFormat('2026-06-11', 'us-mmmm')).toBe('June 11, 2026');
    expect(formatDateWithFormat('2026-06-11', 'eu-mmm')).toBe('11 Jun, 2026');
    expect(formatDateWithFormat('2026-06-11', 'eu-mmmm')).toBe('11 June, 2026');
  });

  test('returns empty string for empty input', () => {
    expect(formatDateWithFormat('', 'eu')).toBe('');
  });

  test('returns unparseable input unchanged', () => {
    expect(formatDateWithFormat('not-a-date', 'eu')).toBe('not-a-date');
  });
});

describe('formatBirthdayWithFormat', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 5, 11)); // 2026-06-11
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('formats full birthdays in EU and US styles', () => {
    expect(formatBirthdayWithFormat('1990-04-30', 'eu')).toBe('30.04.1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'us')).toBe('04/30/1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'iso')).toBe('1990-04-30');
  });

  test('formats full birthdays in the built-out numeric formats', () => {
    expect(formatBirthdayWithFormat('1990-04-30', 'ca')).toBe('30/04/1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'eu-hyphen')).toBe('30-04-1990');
  });

  test('formats full birthdays in the month-name formats', () => {
    expect(formatBirthdayWithFormat('1990-04-30', 'us-mmm')).toBe('Apr 30, 1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'us-mmmm')).toBe('April 30, 1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'eu-mmm')).toBe('30 Apr, 1990');
    expect(formatBirthdayWithFormat('1990-04-30', 'eu-mmmm')).toBe('30 April, 1990');
  });

  test('formats year-less birthdays', () => {
    expect(formatBirthdayWithFormat('--04-30', 'eu')).toBe('30.04.');
    expect(formatBirthdayWithFormat('--04-30', 'us')).toBe('04/30');
    expect(formatBirthdayWithFormat('--04-30', 'iso')).toBe('04-30');
    expect(formatBirthdayWithFormat('--04-30', 'ca')).toBe('30/04');
    expect(formatBirthdayWithFormat('--04-30', 'eu-hyphen')).toBe('30-04');
    expect(formatBirthdayWithFormat('--04-30', 'us-mmm')).toBe('Apr 30');
    expect(formatBirthdayWithFormat('--04-30', 'us-mmmm')).toBe('April 30');
    expect(formatBirthdayWithFormat('--04-30', 'eu-mmm')).toBe('30 Apr');
    expect(formatBirthdayWithFormat('--04-30', 'eu-mmmm')).toBe('30 April');
  });

  test('appends the age when requested', () => {
    expect(formatBirthdayWithFormat('1990-04-30', 'eu', true)).toBe('30.04.1990 (36)');
    expect(formatBirthdayWithFormat('1990-04-30', 'us', true)).toBe('04/30/1990 (36)');
    expect(formatBirthdayWithFormat('1990-04-30', 'iso', true)).toBe('1990-04-30 (36)');
    expect(formatBirthdayWithFormat('1990-04-30', 'us-mmm', true)).toBe('Apr 30, 1990 (36)');
  });

  test('age accounts for a birthday later in the year', () => {
    expect(formatBirthdayWithFormat('1990-12-31', 'eu', true)).toBe('31.12.1990 (35)');
    expect(formatBirthdayWithFormat('1990-12-31', 'us', true)).toBe('12/31/1990 (35)');
    expect(formatBirthdayWithFormat('1990-12-31', 'iso', true)).toBe('1990-12-31 (35)');
  });

  test('returns unrecognized values unchanged', () => {
    expect(formatBirthdayWithFormat('whenever', 'eu')).toBe('whenever');
  });
});

describe('calculateAgeFromBirthday', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 5, 11)); // 2026-06-11
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('calculates age for a past birthday this year', () => {
    expect(calculateAgeFromBirthday('1990-04-30')).toBe(36);
  });

  test('calculates age for an upcoming birthday this year', () => {
    expect(calculateAgeFromBirthday('1990-12-31')).toBe(35);
  });

  test('returns null for year-less and invalid birthdays', () => {
    expect(calculateAgeFromBirthday('--04-30')).toBeNull();
    expect(calculateAgeFromBirthday('')).toBeNull();
    expect(calculateAgeFromBirthday('30.04.1990')).toBeNull();
  });

  test('returns null for future birth years', () => {
    expect(calculateAgeFromBirthday('2030-01-01')).toBeNull();
  });
});

describe('formatBirthdayForInputWithFormat', () => {
  test('converts ISO birthdays to the editable display format', () => {
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'eu')).toBe('30.04.1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'us')).toBe('04/30/1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'iso')).toBe('1990-04-30');
    expect(formatBirthdayForInputWithFormat('--04-30', 'eu')).toBe('30.04.');
    expect(formatBirthdayForInputWithFormat('--04-30', 'us')).toBe('04/30');
    expect(formatBirthdayForInputWithFormat('--04-30', 'iso')).toBe('04-30');
  });

  test('converts ISO birthdays in the built-out numeric formats', () => {
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'ca')).toBe('30/04/1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'eu-hyphen')).toBe('30-04-1990');
    expect(formatBirthdayForInputWithFormat('--04-30', 'ca')).toBe('30/04');
    expect(formatBirthdayForInputWithFormat('--04-30', 'eu-hyphen')).toBe('30-04');
  });

  test('month-name formats keep the numeric input form', () => {
    // Input stays digit-driven (see parseBirthdayInputWithFormat); the pretty
    // month-name rendering is the read display, not the edit field.
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'us-mmm')).toBe('04/30/1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'us-mmmm')).toBe('04/30/1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'eu-mmm')).toBe('30.04.1990');
    expect(formatBirthdayForInputWithFormat('1990-04-30', 'eu-mmmm')).toBe('30.04.1990');
    expect(formatBirthdayForInputWithFormat('--04-30', 'us-mmm')).toBe('04/30');
    expect(formatBirthdayForInputWithFormat('--04-30', 'eu-mmm')).toBe('30.04.');
  });

  test('returns empty and unrecognized input unchanged', () => {
    expect(formatBirthdayForInputWithFormat('', 'eu')).toBe('');
    expect(formatBirthdayForInputWithFormat('whenever', 'eu')).toBe('whenever');
  });
});

describe('parseBirthdayInputWithFormat', () => {
  test('parses EU full dates and pads day/month', () => {
    expect(parseBirthdayInputWithFormat('30.04.1990', 'eu')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('1.4.1990', 'eu')).toBe('1990-04-01');
  });

  test('parses EU year-less dates with and without trailing dot', () => {
    expect(parseBirthdayInputWithFormat('30.04.', 'eu')).toBe('--04-30');
    expect(parseBirthdayInputWithFormat('30.04', 'eu')).toBe('--04-30');
  });

  test('parses US full and year-less dates', () => {
    expect(parseBirthdayInputWithFormat('04/30/1990', 'us')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('04/30', 'us')).toBe('--04-30');
  });

  test('parses the built-out numeric formats', () => {
    expect(parseBirthdayInputWithFormat('30/04/1990', 'ca')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('30/04', 'ca')).toBe('--04-30');
    expect(parseBirthdayInputWithFormat('30-04-1990', 'eu-hyphen')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('30-04', 'eu-hyphen')).toBe('--04-30');
  });

  test('month-name formats parse their numeric input form', () => {
    expect(parseBirthdayInputWithFormat('04/30/1990', 'us-mmm')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('04/30', 'us-mmm')).toBe('--04-30');
    expect(parseBirthdayInputWithFormat('30.04.1990', 'eu-mmm')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('30.04.', 'eu-mmm')).toBe('--04-30');
  });

  test('accepts ISO input directly regardless of format', () => {
    expect(parseBirthdayInputWithFormat('1990-04-30', 'eu')).toBe('1990-04-30');
    expect(parseBirthdayInputWithFormat('--04-30', 'us')).toBe('--04-30');
    expect(parseBirthdayInputWithFormat('1990-04-30', 'us-mmm')).toBe('1990-04-30');
  });

  test('returns empty string for empty input', () => {
    expect(parseBirthdayInputWithFormat('', 'eu')).toBe('');
    expect(parseBirthdayInputWithFormat('   ', 'eu')).toBe('');
  });

  test('rejects out-of-range and malformed input', () => {
    expect(parseBirthdayInputWithFormat('32.01.1990', 'eu')).toBeNull();
    expect(parseBirthdayInputWithFormat('30.13.1990', 'eu')).toBeNull();
    expect(parseBirthdayInputWithFormat('13/30/1990', 'us')).toBeNull();
    expect(parseBirthdayInputWithFormat('13/31/1990', 'us')).toBeNull();
    expect(parseBirthdayInputWithFormat('1990-13-31', 'iso')).toBeNull();
    expect(parseBirthdayInputWithFormat('hello', 'eu')).toBeNull();
    // Wrong separator for the active format
    expect(parseBirthdayInputWithFormat('04/30/1990', 'eu')).toBeNull();
    expect(parseBirthdayInputWithFormat('30/04/1990', 'eu-hyphen')).toBeNull();
  });
});

describe('autoFormatBirthdayInputWithFormat', () => {
  test('inserts separators while typing in EU format', () => {
    expect(autoFormatBirthdayInputWithFormat('3', '', 'eu')).toBe('3');
    expect(autoFormatBirthdayInputWithFormat('30', '3', 'eu')).toBe('30');
    expect(autoFormatBirthdayInputWithFormat('304', '30', 'eu')).toBe('30.4');
    expect(autoFormatBirthdayInputWithFormat('30.41', '30.4', 'eu')).toBe('30.41');
    expect(autoFormatBirthdayInputWithFormat('30.411', '30.41', 'eu')).toBe('30.41.1');
  });

  test('uses slashes in US format', () => {
    expect(autoFormatBirthdayInputWithFormat('043', '04', 'us')).toBe('04/3');
  });

  test('uses slashes in the Canada day-first format', () => {
    expect(autoFormatBirthdayInputWithFormat('043', '04', 'ca')).toBe('04/3');
    expect(autoFormatBirthdayInputWithFormat('04301', '0430', 'ca')).toBe('04/30/1');
  });

  test('uses hyphens in the European hyphenated format', () => {
    expect(autoFormatBirthdayInputWithFormat('043', '04', 'eu-hyphen')).toBe('04-3');
    expect(autoFormatBirthdayInputWithFormat('04301', '0430', 'eu-hyphen')).toBe('04-30-1');
  });

  test('month-name formats auto-format with their numeric separator', () => {
    expect(autoFormatBirthdayInputWithFormat('043', '04', 'us-mmm')).toBe('04/3');
    expect(autoFormatBirthdayInputWithFormat('043', '04', 'eu-mmm')).toBe('04.3');
  });

  test('strips a trailing separator after deleting a digit', () => {
    expect(autoFormatBirthdayInputWithFormat('30.', '30.4', 'eu')).toBe('30');
  });

  test('keeps a manually typed separator at a boundary', () => {
    expect(autoFormatBirthdayInputWithFormat('30.', '30', 'eu')).toBe('30.');
  });

  test('leaves an ISO year untouched while it is being typed', () => {
    expect(autoFormatBirthdayInputWithFormat('1', '', 'iso')).toBe('1');
    expect(autoFormatBirthdayInputWithFormat('19', '1', 'iso')).toBe('19');
    expect(autoFormatBirthdayInputWithFormat('199', '19', 'iso')).toBe('199');
    expect(autoFormatBirthdayInputWithFormat('1990', '199', 'iso')).toBe('1990');
    expect(autoFormatBirthdayInputWithFormat('19900', '1990', 'iso')).toBe('1990-0');
    expect(autoFormatBirthdayInputWithFormat('1990-04', '1990-0', 'iso')).toBe('1990-04');
    expect(autoFormatBirthdayInputWithFormat('1990-043', '1990-04', 'iso')).toBe('1990-04-3');
    expect(autoFormatBirthdayInputWithFormat('1990-04-30', '1990-04-3', 'iso')).toBe('1990-04-30');
  });

  test('preserves manually typed ISO separators, including year-less MM-DD', () => {
    expect(autoFormatBirthdayInputWithFormat('04-', '04', 'iso')).toBe('04-');
    expect(autoFormatBirthdayInputWithFormat('04-3', '04-', 'iso')).toBe('04-3');
    expect(autoFormatBirthdayInputWithFormat('04-30', '04-3', 'iso')).toBe('04-30');
    expect(autoFormatBirthdayInputWithFormat('1990-', '1990', 'iso')).toBe('1990-');
    expect(autoFormatBirthdayInputWithFormat('1990-04-', '1990-04', 'iso')).toBe('1990-04-');
  });

  test('strips a trailing ISO separator after deleting a digit', () => {
    expect(autoFormatBirthdayInputWithFormat('1990-', '1990-0', 'iso')).toBe('1990');
  });
});
