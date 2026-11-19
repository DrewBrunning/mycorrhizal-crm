import {
  Alert,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { OccasionEvent, OccasionEventInput } from '../api/occasionEvents';
import type { OccasionSensitivity } from '../api/occasionObligations';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import AppDialog from './AppDialog';

interface OccasionEventDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (input: OccasionEventInput) => Promise<void>;
  event?: OccasionEvent | null;
}

// toDatetimeLocal renders an RFC 3339 instant as the value a
// <input type="datetime-local"> expects (local wall clock, no zone suffix).
export function toDatetimeLocal(iso: string | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// fromDatetimeLocal converts a <input type="datetime-local"> value back to an
// RFC 3339 instant, or null when empty/unparseable.
export function fromDatetimeLocal(value: string): string | null {
  if (!value) return null;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

// The event create/edit form (docs/adrs/0026-occasions-events.md, issue #1228).
// Attendees are managed by the separate attendees dialog, never here.
export default function OccasionEventDialog({
  open,
  onClose,
  onSave,
  event,
}: OccasionEventDialogProps) {
  const { t } = useTranslation();
  const [title, setTitle] = useState('');
  const [startsAt, setStartsAt] = useState('');
  const [endsAt, setEndsAt] = useState('');
  const [location, setLocation] = useState('');
  const [sensitivity, setSensitivity] = useState<OccasionSensitivity>('normal');
  const [notes, setNotes] = useState('');
  const [saving, setSaving] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<
    Partial<Record<'title' | 'startsAt' | 'endsAt', string>>
  >({});
  const [formError, setFormError] = useState('');
  // Bumped when a rejected save should move focus to the first invalid field.
  const [focusTick, setFocusTick] = useState(0);
  const contentRef = useRef<HTMLDivElement>(null);
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (open) {
      setTitle(event?.title || '');
      setStartsAt(toDatetimeLocal(event?.starts_at));
      setEndsAt(toDatetimeLocal(event?.ends_at));
      setLocation(event?.location || '');
      setSensitivity(event?.sensitivity || 'normal');
      setNotes(event?.notes || '');
      setFieldErrors({});
      setFormError('');
    }
  }, [open, event]);

  // Issue #2672: focus the first invalid field so its error is announced.
  useEffect(() => {
    if (focusTick === 0) return;
    contentRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
  }, [focusTick]);

  const clearField = (field: 'title' | 'startsAt' | 'endsAt') => {
    setFieldErrors((prev) => {
      if (!(field in prev)) return prev;
      const { [field]: _removed, ...rest } = prev;
      return rest;
    });
    setFormError('');
  };

  const handleSave = async () => {
    const trimmedTitle = title.trim();
    const errors: typeof fieldErrors = {};
    if (!trimmedTitle) errors.title = t('occasions.events.validation.titleRequired');
    const startsIso = fromDatetimeLocal(startsAt);
    if (!startsIso) errors.startsAt = t('occasions.events.validation.startRequired');
    const endsIso = fromDatetimeLocal(endsAt);
    if (startsIso && endsIso && new Date(endsIso) < new Date(startsIso)) {
      errors.endsAt = t('occasions.events.validation.endBeforeStart');
    }
    if (Object.keys(errors).length > 0 || !startsIso) {
      setFieldErrors(errors);
      setFormError('');
      setFocusTick((n) => n + 1);
      return;
    }
    setFieldErrors({});
    setFormError('');

    setSaving(true);
    try {
      await onSave({
        title: trimmedTitle,
        starts_at: startsIso,
        ends_at: endsIso,
        location: location.trim() || undefined,
        sensitivity,
        notes: notes.trim() || undefined,
      });
      onClose();
    } catch {
      setFormError(t('occasions.events.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {event ? t('occasions.events.editTitle') : t('occasions.events.createTitle')}
      </DialogTitle>
      <DialogContent ref={contentRef}>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('occasions.events.dialogHint')}
          </Typography>
          <TextField
            label={t('occasions.events.titleLabel')}
            value={title}
            onChange={(e) => {
              markDirty();
              setTitle(e.target.value);
              clearField('title');
            }}
            fullWidth
            required
            error={fieldErrors.title !== undefined}
            helperText={fieldErrors.title}
          />

          <TextField
            label={t('occasions.events.startsAt')}
            type="datetime-local"
            value={startsAt}
            onChange={(e) => {
              markDirty();
              setStartsAt(e.target.value);
              clearField('startsAt');
            }}
            fullWidth
            required
            error={fieldErrors.startsAt !== undefined}
            helperText={fieldErrors.startsAt}
            slotProps={{ inputLabel: { shrink: true } }}
          />

          <TextField
            label={t('occasions.events.endsAt')}
            type="datetime-local"
            value={endsAt}
            onChange={(e) => {
              markDirty();
              setEndsAt(e.target.value);
              clearField('endsAt');
            }}
            fullWidth
            error={fieldErrors.endsAt !== undefined}
            helperText={fieldErrors.endsAt}
            slotProps={{ inputLabel: { shrink: true } }}
          />

          <TextField
            label={t('occasions.events.location')}
            value={location}
            onChange={(e) => {
              markDirty();
              setLocation(e.target.value);
            }}
            fullWidth
          />

          <TextField
            select
            label={t('occasions.events.sensitivity')}
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
            label={t('occasions.events.notes')}
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
