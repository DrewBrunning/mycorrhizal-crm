import CalendarMonthIcon from '@mui/icons-material/CalendarMonth';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import {
  Box,
  Button,
  IconButton,
  InputAdornment,
  Popover,
  type SxProps,
  TextField,
  type Theme,
  Typography,
} from '@mui/material';
import { useId, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDateFormat } from '../DateFormatProvider';
import { localISODate } from '../utils/localDate';

interface DateFieldProps {
  label: string;
  /** A `YYYY-MM-DD` calendar day, or '' for no selection. */
  value: string;
  onChange: (value: string) => void;
  id?: string;
  min?: string;
  max?: string;
  size?: 'small' | 'medium';
  /** Marks the field as required (MUI appends " *" to the label). */
  required?: boolean;
  /**
   * An error the caller wants shown on this field (e.g. a server field error);
   * the field's own typed-text errors take precedence while they are showing.
   */
  errorText?: string;
  sx?: SxProps<Theme>;
}

const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

function parseISODate(value: string): Date | null {
  const match = ISO_DATE.exec(value);
  if (!match) return null;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return Number.isNaN(date.getTime()) ? null : date;
}

const FULL_ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

/** True for a real calendar day (rejects 31 February), in a 4-digit year. */
function isRealCalendarDay(iso: string): boolean {
  if (!FULL_ISO_DATE.test(iso)) return false;
  const [year, month, day] = iso.split('-').map(Number);
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  return date.getUTCMonth() === month - 1 && date.getUTCDate() === day;
}

type TypedDate = { iso: string } | { error: 'invalid' | 'range' };

// Intl.Locale#weekInfo is not in every engine's TypeScript lib yet.
interface WeekInfo {
  firstDay?: number;
}

function firstWeekday(locale: string): number {
  try {
    const info = (
      new Intl.Locale(locale) as Intl.Locale & { weekInfo?: WeekInfo; getWeekInfo?: () => WeekInfo }
    ).weekInfo;
    if (info?.firstDay) return info.firstDay;
    const getInfo = (new Intl.Locale(locale) as Intl.Locale & { getWeekInfo?: () => WeekInfo })
      .getWeekInfo;
    const dynamic = getInfo?.();
    if (dynamic?.firstDay) return dynamic.firstDay;
  } catch {
    // Older engine: fall back to Monday (the app's default date format is EU).
  }
  return 1;
}

// A month grid acknowledges the locale's first day of the week. `firstDay`
// uses Intl's 1=Monday … 7=Sunday numbering.
function monthCells(year: number, month: number, firstDay: number): (string | null)[] {
  const mondayIndex = (new Date(year, month, 1).getDay() + 6) % 7; // 0 = Monday
  const offset = (mondayIndex - (firstDay - 1) + 7) % 7;
  const total = new Date(year, month + 1, 0).getDate();
  const cells: (string | null)[] = [];
  for (let i = 0; i < offset; i++) cells.push(null);
  for (let day = 1; day <= total; day++) {
    cells.push(localISODate(new Date(year, month, day)));
  }
  // Always six week rows: a month that needs fewer would shrink the popup and
  // move the previous/next buttons out from under the cursor (issue #2199).
  while (cells.length < 42) cells.push(null);
  return cells;
}

/**
 * A date input that shows the picked calendar day in the user's Date format
 * preference and opens a month calendar (issue #1789). A browser-native
 * `<input type="date">` renders `mm/dd/yyyy` regardless of the app preference
 * and, on WebKit, looks like an applied filter even when empty.
 *
 * The text box is editable: a date typed in the preferred format (issue #2198)
 * updates the value, and invalid text shows an error and leaves the value
 * unchanged. The calendar is an add-on opened from the icon button.
 *
 * The value contract is a date-only `YYYY-MM-DD`, the same as the native input
 * it replaces.
 */
export default function DateField({
  label,
  value,
  onChange,
  id,
  min,
  max,
  size = 'small',
  required = false,
  errorText,
  sx,
}: DateFieldProps) {
  const { t, i18n } = useTranslation();
  const { formatDate, formatBirthdayForInput, parseBirthdayInput, getDatePlaceholder } =
    useDateFormat();
  const errorId = useId();
  // The text being typed, or null when the field mirrors `value`.
  const [draft, setDraft] = useState<string | null>(null);
  const [focused, setFocused] = useState(false);
  const [error, setError] = useState<'invalid' | 'range' | null>(null);
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const anchorRef = useRef<HTMLDivElement | null>(null);
  const selected = parseISODate(value);
  const [view, setView] = useState(() => selected ?? new Date());
  const open = Boolean(anchorEl);

  const locale = i18n.language;
  const firstDay = useMemo(() => firstWeekday(locale), [locale]);
  const monthLabel = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric' }).format(
        new Date(view.getFullYear(), view.getMonth(), 1),
      ),
    [locale, view],
  );
  const weekdayLabels = useMemo(() => {
    const formatter = new Intl.DateTimeFormat(locale, { weekday: 'short' });
    // 2024-01-01 is a Monday, so this walks the week from the locale's first
    // day without depending on "today".
    return Array.from({ length: 7 }, (_, index) => {
      const day = (firstDay - 1 + index) % 7;
      return formatter.format(new Date(2024, 0, 1 + day));
    });
  }, [firstDay, locale]);
  const cells = useMemo(
    () => monthCells(view.getFullYear(), view.getMonth(), firstDay),
    [view, firstDay],
  );
  const today = localISODate(new Date());

  const openPicker = () => {
    // An empty field opens on its earliest allowed month (e.g. an end date opens
    // on the start's month), otherwise on today's.
    setView(selected ?? parseISODate(min ?? '') ?? new Date());
    setAnchorEl(anchorRef.current);
  };

  const close = () => setAnchorEl(null);

  const choose = (iso: string) => {
    setDraft(null);
    setError(null);
    onChange(iso);
    close();
  };

  const moveMonth = (delta: number) => {
    setView((current) => new Date(current.getFullYear(), current.getMonth() + delta, 1));
  };

  const disabled = (iso: string) => (min != null && iso < min) || (max != null && iso > max);

  // Typed text must be a complete, real calendar day in the user's Date
  // format. A year-less entry ("15.01.") is not a valid value for this field.
  const parseTyped = (text: string): TypedDate => {
    const parsed = parseBirthdayInput(text);
    if (parsed === null || !isRealCalendarDay(parsed)) return { error: 'invalid' };
    return disabled(parsed) ? { error: 'range' } : { iso: parsed };
  };

  const handleTyping = (text: string) => {
    setDraft(text);
    setError(null);
    // Report a complete valid date (or an emptied field) as it is typed; the
    // text itself is only normalized on blur so the caret is never moved.
    if (text.trim() === '') {
      onChange('');
      return;
    }
    const typed = parseTyped(text);
    if ('iso' in typed && typed.iso !== value) onChange(typed.iso);
  };

  // Settle what was typed: a valid date is shown in the preferred format, an
  // invalid one keeps its text, shows an error and never touches the value.
  const settle = () => {
    if (draft === null) return;
    if (draft.trim() === '') {
      setDraft(null);
      return;
    }
    const typed = parseTyped(draft);
    if ('error' in typed) {
      setError(typed.error);
      return;
    }
    if (typed.iso !== value) onChange(typed.iso);
    setDraft(null);
  };

  const shown =
    draft ?? (selected ? (focused ? formatBirthdayForInput(value) : formatDate(value)) : '');
  const helperText =
    error === 'range'
      ? t('common.dateOutOfRange')
      : error === 'invalid'
        ? t('common.dateInvalid', { format: getDatePlaceholder() })
        : errorText;

  return (
    <Box ref={anchorRef} sx={sx ?? { width: '100%' }}>
      <TextField
        id={id}
        label={label}
        value={shown}
        placeholder={getDatePlaceholder()}
        size={size}
        fullWidth
        required={required}
        error={error !== null || Boolean(errorText)}
        helperText={helperText}
        onChange={(event) => handleTyping(event.target.value)}
        onFocus={() => setFocused(true)}
        onBlur={() => {
          setFocused(false);
          settle();
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && draft !== null) {
            // Commit typed text instead of submitting a surrounding form with
            // a half-validated value.
            event.preventDefault();
            settle();
          } else if (event.key === 'ArrowDown') {
            event.preventDefault();
            openPicker();
          }
        }}
        slotProps={{
          input: {
            endAdornment: (
              <InputAdornment position="end">
                <IconButton
                  size="small"
                  edge="end"
                  aria-label={t('common.openCalendar')}
                  aria-haspopup="dialog"
                  onClick={openPicker}
                >
                  <CalendarMonthIcon fontSize="small" />
                </IconButton>
              </InputAdornment>
            ),
          },
          htmlInput: {
            'aria-haspopup': 'dialog',
            'aria-label': required ? `${label} *` : label,
            'aria-required': required ? 'true' : undefined,
            'aria-describedby': helperText ? errorId : undefined,
          },
          formHelperText: { id: errorId },
        }}
      />

      <Popover
        open={open}
        anchorEl={anchorEl}
        onClose={close}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      >
        <Box
          role="dialog"
          aria-label={label}
          sx={{ p: 1.5, width: 288 }}
          onKeyDown={(event) => {
            if (event.key === 'Escape') close();
          }}
        >
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
            <IconButton
              size="small"
              onClick={() => moveMonth(-1)}
              aria-label={t('common.previousMonth')}
            >
              <ChevronLeftIcon />
            </IconButton>
            <Typography variant="subtitle2" sx={{ flex: 1, textAlign: 'center' }}>
              {monthLabel}
            </Typography>
            <IconButton
              size="small"
              onClick={() => moveMonth(1)}
              aria-label={t('common.nextMonth')}
            >
              <ChevronRightIcon />
            </IconButton>
          </Box>

          <Box
            component="div"
            sx={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 0.25 }}
          >
            {weekdayLabels.map((weekday) => (
              <Typography
                key={weekday}
                variant="caption"
                sx={{ textAlign: 'center', color: 'text.secondary', py: 0.25 }}
              >
                {weekday}
              </Typography>
            ))}
            {cells.map((iso, index) =>
              iso === null ? (
                <Box
                  // biome-ignore lint/suspicious/noArrayIndexKey: fixed 7-column grid
                  key={`blank-${index}`}
                  data-cell=""
                  sx={{ height: 34 }}
                />
              ) : (
                <Box
                  component="button"
                  type="button"
                  key={iso}
                  data-date={iso}
                  data-cell=""
                  aria-label={formatDate(iso)}
                  aria-pressed={value === iso}
                  disabled={disabled(iso)}
                  onClick={() => choose(iso)}
                  sx={{
                    border: iso === today ? 1 : 0,
                    borderColor: 'primary.main',
                    borderRadius: '50%',
                    width: 34,
                    height: 34,
                    p: 0,
                    font: 'inherit',
                    fontSize: '0.875rem',
                    cursor: 'pointer',
                    color: 'inherit',
                    bgcolor: value === iso ? 'primary.main' : 'transparent',
                    '&:hover': { bgcolor: value === iso ? 'primary.dark' : 'action.hover' },
                    '&.Mui-disabled, &:disabled': { opacity: 0.35, cursor: 'default' },
                    ...(value === iso && { color: 'primary.contrastText' }),
                  }}
                >
                  {Number(iso.slice(8, 10))}
                </Box>
              ),
            )}
          </Box>

          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 1 }}>
            <Button size="small" onClick={() => choose('')} disabled={!value}>
              {t('common.clear')}
            </Button>
          </Box>
        </Box>
      </Popover>
    </Box>
  );
}
