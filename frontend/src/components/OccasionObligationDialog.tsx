import {
  Alert,
  Autocomplete,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api/client';
import {
  OCCASION_OBLIGATION_KINDS,
  type OccasionObligation,
  type OccasionObligationInput,
  type OccasionSensitivity,
} from '../api/occasionObligations';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import { localizeReason } from '../i18n/reason';
import AppDialog from './AppDialog';

export interface OccasionObligationFormData {
  kind: string;
  label: string;
  anchorMonth: number | null;
  anchorDay: number | null;
  leadTimeDays: number;
  active: boolean;
  sensitivity: OccasionSensitivity;
  notes?: string;
}

interface OccasionObligationDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: OccasionObligationFormData) => Promise<void>;
  obligation?: OccasionObligation | null;
}

const MONTHS = Array.from({ length: 12 }, (_, i) => i + 1);
// Longest each month gets in a yearly recurrence; February is 29 so a leap-day
// anchor stays expressible. Mirrors the backend's occasionMaxDay table
// (controllers/occasion_obligation_controller.go) -- keep the two in sync.
const MONTH_MAX_DAY = [31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
const maxDayOf = (month: number | ''): number => (month === '' ? 31 : MONTH_MAX_DAY[month - 1]);

// A field's validation message; '' marks the field invalid with no text of its own.
type FieldErrors = Partial<Record<'label' | 'month' | 'day' | 'leadTime', string>>;

// The server's reason codes for a rejected anchor date, and the field each names.
const ANCHOR_REASON_FIELD: Record<string, 'month' | 'day'> = {
  occasion_anchor_date_required: 'month',
  occasion_anchor_date_incomplete: 'day',
  occasion_anchor_date_invalid: 'day',
};

export default function OccasionObligationDialog({
  open,
  onClose,
  onSave,
  obligation,
}: OccasionObligationDialogProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState('');
  const [label, setLabel] = useState('');
  const [anchorMonth, setAnchorMonth] = useState<number | ''>('');
  const [anchorDay, setAnchorDay] = useState<number | ''>('');
  const [leadTimeDays, setLeadTimeDays] = useState('0');
  const [active, setActive] = useState(true);
  const [sensitivity, setSensitivity] = useState<OccasionSensitivity>('normal');
  const [notes, setNotes] = useState('');
  const [saving, setSaving] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState('');
  const [dayNotice, setDayNotice] = useState('');
  // Bumped whenever a rejected save should move focus to the first invalid control.
  const [focusTick, setFocusTick] = useState(0);
  const contentRef = useRef<HTMLDivElement>(null);
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (open) {
      setKind(obligation?.kind || 'card');
      setLabel(obligation?.label || '');
      setAnchorMonth(obligation?.anchor_month ?? '');
      setAnchorDay(obligation?.anchor_day ?? '');
      setLeadTimeDays(obligation ? String(obligation.lead_time_days) : '0');
      setActive(obligation ? obligation.active : true);
      setSensitivity(obligation?.sensitivity || 'normal');
      setNotes(obligation?.notes || '');
      setFieldErrors({});
      setFormError('');
      setDayNotice('');
    }
  }, [open, obligation]);

  // Issue #2672: after a rejected save, focus the first invalid control so a
  // screen-reader user lands on the field and hears its error.
  useEffect(() => {
    if (focusTick === 0) return;
    contentRef.current
      ?.querySelector<HTMLElement>('[aria-invalid="true"]:not([aria-hidden="true"])')
      ?.focus();
  }, [focusTick]);

  const reject = (errors: FieldErrors) => {
    setFieldErrors(errors);
    setFormError('');
    setFocusTick((n) => n + 1);
  };
  const clearField = (field: keyof FieldErrors) => {
    setFieldErrors((prev) => {
      if (!(field in prev)) return prev;
      const { [field]: _removed, ...rest } = prev;
      return rest;
    });
    setFormError('');
  };

  const handleMonthChange = (value: number | '') => {
    markDirty();
    setAnchorMonth(value);
    clearField('month');
    const max = maxDayOf(value);
    if (anchorDay !== '' && anchorDay > max) {
      setAnchorDay('');
      setDayNotice(
        t('occasions.obligation.dayClamped', { month: t(`occasions.months.${value}`), max }),
      );
    } else {
      setDayNotice('');
    }
  };

  const handleSave = async () => {
    const trimmedLabel = label.trim();
    const errors: FieldErrors = {};
    if (!trimmedLabel) errors.label = t('occasions.obligation.validation.labelRequired');
    // A row saved before dates were required may still be retired (saved
    // inactive, still dateless); anything else needs a real month and day,
    // matching the server (ADR 0024, issues #2670 #2671).
    const legacyRetire =
      obligation != null &&
      obligation.anchor_month == null &&
      obligation.anchor_day == null &&
      !active;
    if (anchorMonth === '' && anchorDay === '') {
      if (!legacyRetire) {
        errors.month = t('errors.reason.occasion_anchor_date_required');
        errors.day = '';
      }
    } else if (anchorMonth === '') {
      errors.month = t('errors.reason.occasion_anchor_date_incomplete');
    } else if (anchorDay === '') {
      errors.day = t('errors.reason.occasion_anchor_date_incomplete');
    } else if (anchorDay > maxDayOf(anchorMonth)) {
      errors.day = t('errors.reason.occasion_anchor_date_invalid');
    }
    const leadTime = leadTimeDays.trim() === '' ? 0 : parseInt(leadTimeDays, 10);
    if (Number.isNaN(leadTime) || leadTime < 0) {
      errors.leadTime = t('occasions.obligation.validation.invalidLeadTime');
    }
    if (Object.keys(errors).length > 0) {
      reject(errors);
      return;
    }
    setFieldErrors({});
    setFormError('');
    setDayNotice('');

    setSaving(true);
    try {
      await onSave({
        kind: kind.trim() || 'card',
        label: trimmedLabel,
        anchorMonth: anchorMonth === '' ? null : anchorMonth,
        anchorDay: anchorDay === '' ? null : anchorDay,
        leadTimeDays: leadTime,
        active,
        sensitivity,
        notes: notes.trim() || undefined,
      });
      onClose();
    } catch (err) {
      // Show the server's own reason, localized, rather than the generic
      // failure: on the field it names when it is an anchor-date rejection.
      const reason = err instanceof ApiError ? err.reasonCode : undefined;
      const localized =
        err instanceof ApiError ? localizeReason(err.reasonCode, err.reasonParams) : undefined;
      const field = reason ? ANCHOR_REASON_FIELD[reason] : undefined;
      if (field && localized) {
        reject({ [field]: localized });
      } else {
        setFormError(localized ?? t('occasions.obligation.validation.saveFailed'));
      }
    } finally {
      setSaving(false);
    }
  };

  // The Day list follows the month; an existing out-of-range day (a row saved
  // before the server checked) stays listed so the select can show it.
  const dayOptions = Array.from(
    { length: Math.max(maxDayOf(anchorMonth), anchorDay === '' ? 0 : anchorDay) },
    (_, i) => i + 1,
  );

  return (
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {obligation ? t('occasions.obligation.editTitle') : t('occasions.obligation.createTitle')}
      </DialogTitle>
      <DialogContent ref={contentRef}>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('occasions.obligation.dialogHint')}
          </Typography>
          <Autocomplete
            freeSolo
            options={[...OCCASION_OBLIGATION_KINDS]}
            value={kind}
            onInputChange={(_e, value) => {
              markDirty();
              setKind(value);
              setFormError('');
            }}
            renderInput={(params) => (
              <TextField
                {...params}
                label={t('occasions.obligation.kind')}
                helperText={t('occasions.obligation.kindHint')}
              />
            )}
          />

          <TextField
            label={t('occasions.obligation.label')}
            value={label}
            onChange={(e) => {
              markDirty();
              setLabel(e.target.value);
              clearField('label');
            }}
            fullWidth
            required
            error={fieldErrors.label !== undefined}
            helperText={fieldErrors.label || t('occasions.obligation.labelHint')}
          />

          <Box sx={{ display: 'flex', gap: 1 }}>
            <TextField
              select
              label={t('occasions.obligation.anchorMonth')}
              value={anchorMonth}
              onChange={(e) =>
                handleMonthChange(e.target.value === '' ? '' : Number(e.target.value))
              }
              fullWidth
              error={fieldErrors.month !== undefined}
              helperText={fieldErrors.month || undefined}
            >
              <MenuItem value="">{t('occasions.obligation.anchorDateUnset')}</MenuItem>
              {MONTHS.map((m) => (
                <MenuItem key={m} value={m}>
                  {t(`occasions.months.${m}`)}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              select
              label={t('occasions.obligation.anchorDay')}
              value={anchorDay}
              onChange={(e) => {
                markDirty();
                setAnchorDay(e.target.value === '' ? '' : Number(e.target.value));
                setDayNotice('');
                clearField('day');
              }}
              fullWidth
              error={fieldErrors.day !== undefined}
              helperText={fieldErrors.day || dayNotice || undefined}
            >
              <MenuItem value="">{t('occasions.obligation.anchorDateUnset')}</MenuItem>
              {dayOptions.map((d) => (
                <MenuItem key={d} value={d}>
                  {d}
                </MenuItem>
              ))}
            </TextField>
          </Box>

          <TextField
            label={t('occasions.obligation.leadTimeDays')}
            type="number"
            value={leadTimeDays}
            onChange={(e) => {
              markDirty();
              setLeadTimeDays(e.target.value);
              clearField('leadTime');
            }}
            fullWidth
            error={fieldErrors.leadTime !== undefined}
            helperText={fieldErrors.leadTime || t('occasions.obligation.leadTimeDaysHint')}
            slotProps={{ htmlInput: { min: 0, max: 365 } }}
          />

          <FormControlLabel
            control={
              <Switch
                checked={active}
                onChange={(e) => {
                  markDirty();
                  setActive(e.target.checked);
                }}
              />
            }
            label={t('occasions.obligation.active')}
          />

          <TextField
            select
            label={t('occasions.obligation.sensitivity')}
            value={sensitivity}
            onChange={(e) => {
              markDirty();
              setSensitivity(e.target.value as OccasionSensitivity);
            }}
            fullWidth
          >
            <MenuItem value="normal">{t('occasions.obligation.sensitivities.normal')}</MenuItem>
            <MenuItem value="private">{t('occasions.obligation.sensitivities.private')}</MenuItem>
            <MenuItem value="secret">{t('occasions.obligation.sensitivities.secret')}</MenuItem>
          </TextField>

          <TextField
            label={t('occasions.obligation.notes')}
            value={notes}
            onChange={(e) => {
              markDirty();
              setNotes(e.target.value);
            }}
            fullWidth
            multiline
            minRows={2}
          />

          {formError && <Alert severity="error">{formError}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={requestClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleSave()} variant="contained" disabled={saving}>
          {t('common.save')}
        </Button>
      </DialogActions>
      {guardElement}
    </AppDialog>
  );
}

// Rebuilds an OccasionObligationInput for the wire from a form payload + the
// target entity.
export function toOccasionObligationInput(
  entityId: string,
  data: OccasionObligationFormData,
): OccasionObligationInput {
  return {
    entity_id: entityId,
    kind: data.kind,
    label: data.label,
    anchor_month: data.anchorMonth,
    anchor_day: data.anchorDay,
    lead_time_days: data.leadTimeDays,
    active: data.active,
    sensitivity: data.sensitivity,
    notes: data.notes,
  };
}
