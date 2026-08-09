import { ReactNode, createContext, useContext, useEffect, useMemo, useState, useCallback } from "react";

// The supported date formats. Keys are persisted backend-side as the user's
// `date_format` preference, so they are a stable contract shared with
// backend/models/user.go, backend/controllers/user_controller.go and
// backend/openapi.yaml — if you add a key here, add it in all three (and in
// the reminder-service email formatters in backend/services/reminder_service.go).
export type DateFormat =
  | "eu"        // DD.MM.YYYY
  | "us"        // MM/DD/YYYY
  | "iso"       // YYYY-MM-DD
  | "ca"        // DD/MM/YYYY
  | "eu-hyphen" // DD-MM-YYYY
  | "us-mmm"    // MMM D, YYYY (en-US, abbreviated month)
  | "us-mmmm"   // MMMM D, YYYY (en-US, full month)
  | "eu-mmm"    // D MMM, YYYY
  | "eu-mmmm";  // DD MMMM, YYYY

// Frontend enum mirrors of the backend `oneof` validator — keep in sync by
// hand (see /CLAUDE.md frontend trap #4).
const SUPPORTED_DATE_FORMATS: DateFormat[] = [
  "eu", "us", "iso", "ca", "eu-hyphen", "us-mmm", "us-mmmm", "eu-mmm", "eu-mmmm",
];

// Month names for the month-name display formats. English by design: these
// formats are the en-US-style "mmm/mmmm" tokens the user asked for, and they
// are a display preference independent of UI language. Input still uses the
// numeric form (see parseBirthdayInputWithFormat) so typing stays digit-driven.
const MONTHS_SHORT = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const MONTHS_LONG = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

interface DateFormatContextValue {
  dateFormat: DateFormat;
  setDateFormat: (format: DateFormat) => void;
  formatDate: (dateString: string) => string;
  formatBirthday: (birthday: string, includeAge?: boolean) => string;
  formatBirthdayForInput: (birthday: string) => string;
  parseBirthdayInput: (input: string) => string | null;
  autoFormatBirthdayInput: (newValue: string, prevValue: string) => string;
  getBirthdayPlaceholder: () => string;
  getBirthdayFormatHint: () => string;
  getDatePlaceholder: () => string;
  calculateAge: (birthday: string) => number | null;
}

const DateFormatContext = createContext<DateFormatContextValue | undefined>(undefined);

const DATE_FORMAT_STORAGE_KEY = "dateFormat";

// Initialize date format from backend value (called on login)
export function initializeDateFormatFromBackend(dateFormat: string | undefined): void {
  if (typeof window === "undefined") {
    return;
  }
  if (dateFormat && SUPPORTED_DATE_FORMATS.includes(dateFormat as DateFormat)) {
    window.localStorage.setItem(DATE_FORMAT_STORAGE_KEY, dateFormat as DateFormat);
  }
}

const getStoredFormat = (): DateFormat => {
  if (typeof window === "undefined") {
    return "eu";
  }

  const storedValue = window.localStorage.getItem(DATE_FORMAT_STORAGE_KEY);
  if (storedValue && SUPPORTED_DATE_FORMATS.includes(storedValue as DateFormat)) {
    return storedValue as DateFormat;
  }

  return "eu";
};

/**
 * Calculate age from a birthday string (YYYY-MM-DD or --MM-DD)
 * Returns null if no year is provided or if the format is invalid
 */
export function calculateAgeFromBirthday(birthday: string): number | null {
  if (!birthday || birthday.startsWith('--')) {
    return null;
  }

  const parts = birthday.split('-');
  if (parts.length !== 3 || parts[0].length !== 4) {
    return null;
  }

  const birthYear = parseInt(parts[0], 10);
  const month = parseInt(parts[1], 10);
  const day = parseInt(parts[2], 10);

  if (isNaN(day) || isNaN(month) || isNaN(birthYear)) {
    return null;
  }

  const today = new Date();
  const currentYear = today.getFullYear();
  const currentMonth = today.getMonth() + 1;
  const currentDay = today.getDate();

  let age = currentYear - birthYear;

  // Adjust if birthday hasn't occurred yet this year
  if (month > currentMonth || (month === currentMonth && day > currentDay)) {
    age--;
  }

  return age >= 0 ? age : null;
}

// Renders a full YYYY/MM/DD (all zero-padded except where the format token
// says otherwise) per the display format.
function formatFullDate(year: string, month: string, day: string, format: DateFormat): string {
  const dayNum = parseInt(day, 10);
  const monthNum = parseInt(month, 10);
  switch (format) {
    case "eu":
      return `${day}.${month}.${year}`;
    case "us":
      return `${month}/${day}/${year}`;
    case "iso":
      return `${year}-${month}-${day}`;
    case "ca":
      return `${day}/${month}/${year}`;
    case "eu-hyphen":
      return `${day}-${month}-${year}`;
    case "us-mmm":
      return `${MONTHS_SHORT[monthNum - 1]} ${dayNum}, ${year}`;
    case "us-mmmm":
      return `${MONTHS_LONG[monthNum - 1]} ${dayNum}, ${year}`;
    case "eu-mmm":
      return `${dayNum} ${MONTHS_SHORT[monthNum - 1]}, ${year}`;
    case "eu-mmmm":
      return `${day} ${MONTHS_LONG[monthNum - 1]}, ${year}`;
  }
}

// Renders a year-less MM/DD per the display format (the birthday "month and
// day only" case; the trailing dot of the eu family is kept).
function formatYearlessDate(month: string, day: string, format: DateFormat): string {
  const dayNum = parseInt(day, 10);
  const monthNum = parseInt(month, 10);
  switch (format) {
    case "eu":
      return `${day}.${month}.`;
    case "us":
      return `${month}/${day}`;
    case "iso":
      return `${month}-${day}`;
    case "ca":
      return `${day}/${month}`;
    case "eu-hyphen":
      return `${day}-${month}`;
    case "us-mmm":
      return `${MONTHS_SHORT[monthNum - 1]} ${dayNum}`;
    case "us-mmmm":
      return `${MONTHS_LONG[monthNum - 1]} ${dayNum}`;
    case "eu-mmm":
      return `${dayNum} ${MONTHS_SHORT[monthNum - 1]}`;
    case "eu-mmmm":
      return `${day} ${MONTHS_LONG[monthNum - 1]}`;
  }
}

/**
 * Format a standard date (ISO format) to the user's preferred display format
 */
export function formatDateWithFormat(dateString: string, format: DateFormat): string {
  if (!dateString) return '';

  const date = new Date(dateString);
  if (isNaN(date.getTime())) return dateString;

  const day = String(date.getUTCDate()).padStart(2, '0');
  const month = String(date.getUTCMonth() + 1).padStart(2, '0');
  const year = date.getUTCFullYear();

  return formatFullDate(String(year), month, day, format);
}

/**
 * Format a birthday (YYYY-MM-DD or --MM-DD) to the user's preferred display format
 * Optionally includes age calculation
 */
export function formatBirthdayWithFormat(birthday: string, format: DateFormat, includeAge: boolean = false): string {
  if (!birthday) return '';

  // Check if it's a year-less birthday (starts with --)
  if (birthday.startsWith('--')) {
    // --MM-DD format
    const month = birthday.substring(2, 4);
    const day = birthday.substring(5, 7);
    return formatYearlessDate(month, day, format);
  }

  // YYYY-MM-DD format
  const parts = birthday.split('-');
  if (parts.length === 3) {
    const year = parts[0];
    const month = parts[1];
    const day = parts[2];

    let dateStr = formatFullDate(year, month, day, format);

    // Calculate age if requested and year is valid
    if (includeAge && year && year.length === 4) {
      const birthYear = parseInt(year, 10);
      if (!isNaN(birthYear)) {
        const today = new Date();
        const birthDate = new Date(birthYear, parseInt(month, 10) - 1, parseInt(day, 10));
        let age = today.getFullYear() - birthYear;

        // Adjust if birthday hasn't occurred yet this year
        if (today < new Date(today.getFullYear(), birthDate.getMonth(), birthDate.getDate())) {
          age--;
        }

        if (age >= 0) {
          return `${dateStr} (${age})`;
        }
      }
    }

    return dateStr;
  }

  return birthday; // Return as-is if format doesn't match
}

// The numeric input pattern for a format. Month-name formats (us-mmm/us-mmmm,
// eu-mmm/eu-mmmm) keep the numeric input of their country's order — users type
// digits, the read display shows the pretty month-name form.
function inputStyle(format: DateFormat): { sep: string; dayFirst: boolean } {
  switch (format) {
    case "eu":
    case "eu-mmm":
    case "eu-mmmm":
      return { sep: ".", dayFirst: true };
    case "us":
    case "us-mmm":
    case "us-mmmm":
      return { sep: "/", dayFirst: false };
    case "ca":
      return { sep: "/", dayFirst: true };
    case "eu-hyphen":
      return { sep: "-", dayFirst: true };
    default: // iso — year first, handled separately
      return { sep: "-", dayFirst: false };
  }
}

/**
 * Format a birthday for editing (convert ISO to the numeric input format)
 */
export function formatBirthdayForInputWithFormat(birthday: string, format: DateFormat): string {
  if (!birthday) return '';

  const isEuFamily = format === "eu" || format === "eu-mmm" || format === "eu-mmmm";
  const isCa = format === "ca";
  const isEuHyphen = format === "eu-hyphen";
  const isIso = format === "iso";

  // Check if it's a year-less birthday (starts with --)
  if (birthday.startsWith('--')) {
    const month = birthday.substring(2, 4);
    const day = birthday.substring(5, 7);

    if (isEuFamily) return `${day}.${month}.`;
    if (isCa) return `${day}/${month}`;
    if (isEuHyphen) return `${day}-${month}`;
    if (isIso) return `${month}-${day}`;
    return `${month}/${day}`; // us, us-mmm, us-mmmm
  }

  // YYYY-MM-DD format
  const parts = birthday.split('-');
  if (parts.length === 3) {
    const year = parts[0];
    const month = parts[1];
    const day = parts[2];

    if (isEuFamily) return `${day}.${month}.${year}`;
    if (isCa) return `${day}/${month}/${year}`;
    if (isEuHyphen) return `${day}-${month}-${year}`;
    if (isIso) return `${year}-${month}-${day}`;
    return `${month}/${day}/${year}`; // us, us-mmm, us-mmmm
  }

  return birthday;
}

// Shared month/day range validation for the numeric parsers.
function isValidDateParts(monthStr: string, dayStr: string): boolean {
  const monthNum = parseInt(monthStr, 10);
  const dayNum = parseInt(dayStr, 10);
  return monthNum >= 1 && monthNum <= 12 && dayNum >= 1 && dayNum <= 31;
}

/**
 * Parse user input in display format back to ISO format for storage
 * Returns null if input is invalid
 */
export function parseBirthdayInputWithFormat(input: string, format: DateFormat): string | null {
  if (!input || input.trim() === '') return '';

  const trimmed = input.trim();

  // Also accept ISO format directly (YYYY-MM-DD or --MM-DD)
  const isoFullDateRegex = /^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$/;
  const isoYearlessRegex = /^--(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$/;
  if (isoFullDateRegex.test(trimmed) || isoYearlessRegex.test(trimmed)) {
    return trimmed;
  }

  const isEuFamily = format === "eu" || format === "eu-mmm" || format === "eu-mmmm";
  const isCa = format === "ca";
  const isEuHyphen = format === "eu-hyphen";

  if (isEuFamily) {
    // EU format: DD.MM.YYYY or DD.MM.
    // Full date with year
    const euFullMatch = trimmed.match(/^(\d{1,2})\.(\d{1,2})\.(\d{4})$/);
    if (euFullMatch) {
      const day = euFullMatch[1].padStart(2, '0');
      const month = euFullMatch[2].padStart(2, '0');
      const year = euFullMatch[3];
      if (!isValidDateParts(month, day)) return null;
      return `${year}-${month}-${day}`;
    }

    // Year-less format: DD.MM. or DD.MM
    const euYearlessMatch = trimmed.match(/^(\d{1,2})\.(\d{1,2})\.?$/);
    if (euYearlessMatch) {
      const day = euYearlessMatch[1].padStart(2, '0');
      const month = euYearlessMatch[2].padStart(2, '0');
      if (!isValidDateParts(month, day)) return null;
      return `--${month}-${day}`;
    }
  } else if (isCa) {
    // Canada: DD/MM/YYYY or DD/MM (day-first, slash separator)
    const caFullMatch = trimmed.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})$/);
    if (caFullMatch) {
      const day = caFullMatch[1].padStart(2, '0');
      const month = caFullMatch[2].padStart(2, '0');
      const year = caFullMatch[3];
      if (!isValidDateParts(month, day)) return null;
      return `${year}-${month}-${day}`;
    }

    const caYearlessMatch = trimmed.match(/^(\d{1,2})\/(\d{1,2})$/);
    if (caYearlessMatch) {
      const day = caYearlessMatch[1].padStart(2, '0');
      const month = caYearlessMatch[2].padStart(2, '0');
      if (!isValidDateParts(month, day)) return null;
      return `--${month}-${day}`;
    }
  } else if (isEuHyphen) {
    // European hyphenated: DD-MM-YYYY or DD-MM (day-first, hyphen separator)
    const ehFullMatch = trimmed.match(/^(\d{1,2})-(\d{1,2})-(\d{4})$/);
    if (ehFullMatch) {
      const day = ehFullMatch[1].padStart(2, '0');
      const month = ehFullMatch[2].padStart(2, '0');
      const year = ehFullMatch[3];
      if (!isValidDateParts(month, day)) return null;
      return `${year}-${month}-${day}`;
    }

    const ehYearlessMatch = trimmed.match(/^(\d{1,2})-(\d{1,2})$/);
    if (ehYearlessMatch) {
      const day = ehYearlessMatch[1].padStart(2, '0');
      const month = ehYearlessMatch[2].padStart(2, '0');
      if (!isValidDateParts(month, day)) return null;
      return `--${month}-${day}`;
    }
  } else {
    // US format (us, us-mmm, us-mmmm) + iso: MM/DD/YYYY or MM/DD (iso also
    // accepts the year-less MM-DD form).
    const usFullMatch = trimmed.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})$/);
    if (usFullMatch) {
      const month = usFullMatch[1].padStart(2, '0');
      const day = usFullMatch[2].padStart(2, '0');
      const year = usFullMatch[3];
      if (!isValidDateParts(month, day)) return null;
      return `${year}-${month}-${day}`;
    }

    // Year-less format: us MM/DD or iso MM-DD
    const usYearlessMatch = trimmed.match(/^(\d{1,2})[-/](\d{1,2})$/);
    if (usYearlessMatch) {
      const month = usYearlessMatch[1].padStart(2, '0');
      const day = usYearlessMatch[2].padStart(2, '0');
      if (!isValidDateParts(month, day)) return null;
      return `--${month}-${day}`;
    }
  }

  return null;
}

export function autoFormatBirthdayInputWithFormat(newValue: string, prevValue: string, format: DateFormat): string {
  const newDigits = newValue.replace(/[^0-9]/g, '');
  const prevDigits = prevValue.replace(/[^0-9]/g, '');

  if (format === 'iso') {
    if (newDigits.length < prevDigits.length) {
      // Digit deleted, so strip leftover trailing separator
      return newValue.replace(/-+$/, '');
    }
    // Up to four digits the input is ambiguous — a year being typed
    // (1990-04-30) or a year-less MM-DD — so leave it exactly as typed.
    if (newDigits.length <= 4) {
      return newValue;
    }
    const formatted =
      newDigits.slice(0, 4) + '-' + newDigits.slice(4, 6) +
      (newDigits.length > 6 ? '-' + newDigits.slice(6, 8) : '');
    // Preserve a separator the user just typed after YYYY-MM.
    if (
      newDigits.length === 6 &&
      newDigits.length === prevDigits.length &&
      newValue.length > prevValue.length &&
      /-$/.test(newValue)
    ) {
      return formatted + '-';
    }
    return formatted;
  }

  const sep = inputStyle(format).sep;

  const formatDigits = (digits: string): string => {
    if (digits.length <= 2) return digits;
    if (digits.length <= 4) return digits.slice(0, 2) + sep + digits.slice(2);
    return digits.slice(0, 2) + sep + digits.slice(2, 4) + sep + digits.slice(4, 8);
  };

  if (newDigits.length < prevDigits.length) {
    // Digit deleted, so strip leftover trailing separator
    return newValue.replace(/[./-]+$/, '');
  }

  if (newDigits.length === prevDigits.length) {
    const formatted = formatDigits(newDigits);
    const atBoundary = newDigits.length === 2 || newDigits.length === 4;
    const endsWithSep = /[./-]$/.test(newValue);
    if (atBoundary && newValue.length > prevValue.length && endsWithSep) {
      return formatted + sep;
    }
    return formatted;
  }

  return formatDigits(newDigits);
}

export function DateFormatProvider({ children }: { children: ReactNode }) {
  const [dateFormat, setDateFormat] = useState<DateFormat>(() => getStoredFormat());

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    window.localStorage.setItem(DATE_FORMAT_STORAGE_KEY, dateFormat);
  }, [dateFormat]);

  const formatDate = useCallback(
    (dateString: string) => formatDateWithFormat(dateString, dateFormat),
    [dateFormat]
  );

  const formatBirthday = useCallback(
    (birthday: string, includeAge: boolean = false) => formatBirthdayWithFormat(birthday, dateFormat, includeAge),
    [dateFormat]
  );

  const formatBirthdayForInput = useCallback(
    (birthday: string) => formatBirthdayForInputWithFormat(birthday, dateFormat),
    [dateFormat]
  );

  const parseBirthdayInput = useCallback(
    (input: string) => parseBirthdayInputWithFormat(input, dateFormat),
    [dateFormat]
  );

  const autoFormatBirthdayInput = useCallback(
    (newValue: string, prevValue: string) =>
      autoFormatBirthdayInputWithFormat(newValue, prevValue, dateFormat),
    [dateFormat]
  );

  const getBirthdayPlaceholder = useCallback(() => {
    switch (dateFormat) {
      case "eu":
      case "eu-mmm":
      case "eu-mmmm":
        return "DD.MM.YYYY";
      case "iso":
        return "YYYY-MM-DD";
      case "ca":
        return "DD/MM/YYYY";
      case "eu-hyphen":
        return "DD-MM-YYYY";
      default: // us, us-mmm, us-mmmm
        return "MM/DD/YYYY";
    }
  }, [dateFormat]);

  const getBirthdayFormatHint = useCallback(() => {
    switch (dateFormat) {
      case "eu":
      case "eu-mmm":
      case "eu-mmmm":
        return "DD.MM.YYYY (year optional, e.g., 30.04.1990 or 30.04.)";
      case "iso":
        return "YYYY-MM-DD (year optional, e.g., 1990-04-30 or --04-30 or 04-30)";
      case "ca":
        return "DD/MM/YYYY (year optional, e.g., 30/04/1990 or 30/04)";
      case "eu-hyphen":
        return "DD-MM-YYYY (year optional, e.g., 30-04-1990 or 30-04)";
      default: // us, us-mmm, us-mmmm
        return "MM/DD/YYYY (year optional, e.g., 04/30/1990 or 04/30)";
    }
  }, [dateFormat]);

  const getDatePlaceholder = useCallback(() => {
    switch (dateFormat) {
      case "eu":
      case "eu-mmm":
      case "eu-mmmm":
        return "DD.MM.YYYY";
      case "iso":
        return "YYYY-MM-DD";
      case "ca":
        return "DD/MM/YYYY";
      case "eu-hyphen":
        return "DD-MM-YYYY";
      default: // us, us-mmm, us-mmmm
        return "MM/DD/YYYY";
    }
  }, [dateFormat]);

  const calculateAge = useCallback(
    (birthday: string) => calculateAgeFromBirthday(birthday),
    []
  );

  const contextValue = useMemo(
    () => ({
      dateFormat,
      setDateFormat,
      formatDate,
      formatBirthday,
      formatBirthdayForInput,
      parseBirthdayInput,
      autoFormatBirthdayInput,
      getBirthdayPlaceholder,
      getBirthdayFormatHint,
      getDatePlaceholder,
      calculateAge,
    }),
    [dateFormat, formatDate, formatBirthday, formatBirthdayForInput, parseBirthdayInput, autoFormatBirthdayInput, getBirthdayPlaceholder, getBirthdayFormatHint, getDatePlaceholder, calculateAge]
  );

  return (
    <DateFormatContext.Provider value={contextValue}>
      {children}
    </DateFormatContext.Provider>
  );
}

export const useDateFormat = () => {
  const context = useContext(DateFormatContext);

  if (!context) {
    throw new Error("useDateFormat must be used within DateFormatProvider");
  }

  return context;
};
