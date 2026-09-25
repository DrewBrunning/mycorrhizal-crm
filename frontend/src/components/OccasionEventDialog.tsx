import {
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { OccasionEvent, OccasionEventInput } from '../api/occasionEvents';
import type { OccasionSensitivity } from '../api/occasionObligations';
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
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setTitle(event?.title || '');
      setStartsAt(toDatetimeLocal(event?.starts_at));
      setEndsAt(toDatetimeLocal(event?.ends_at));
      setLocation(event?.location || '');
      setSensitivity(event?.sensitivity || 'normal');
      setNotes(event?.notes || '');
      setError('');
    }
  }, [open, event]);

  const handleSave = async () => {
    const trimmedTitle = title.trim();
    if (!trimmedTitle) {
      setError(t('occasions.events.validation.titleRequired'));
      return;
    }
    const startsIso = fromDatetimeLocal(startsAt);
    if (!startsIso) {
      setError(t('occasions.events.validation.startRequired'));
      return;
    }
    const endsIso = fromDatetimeLocal(endsAt);
    if (endsIso && new Date(endsIso) < new Date(startsIso)) {
      setError(t('occasions.events.validation.endBeforeStart'));
      return;
    }

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
      setError(t('occasions.events.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {event ? t('occasions.events.editTitle') : t('occasions.events.createTitle')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label={t('occasions.events.titleLabel')}
            value={title}
            onChange={(e) => {
              setTitle(e.target.value);
              setError('');
            }}
            fullWidth
            required
          />

          <TextField
            label={t('occasions.events.startsAt')}
            type="datetime-local"
            value={startsAt}
            onChange={(e) => {
              setStartsAt(e.target.value);
              setError('');
            }}
            fullWidth
            required
            slotProps={{ inputLabel: { shrink: true } }}
          />

          <TextField
            label={t('occasions.events.endsAt')}
            type="datetime-local"
            value={endsAt}
            onChange={(e) => {
              setEndsAt(e.target.value);
              setError('');
            }}
            fullWidth
            slotProps={{ inputLabel: { shrink: true } }}
          />

          <TextField
            label={t('occasions.events.location')}
            value={location}
            onChange={(e) => setLocation(e.target.value)}
            fullWidth
          />

          <TextField
            select
            label={t('occasions.events.sensitivity')}
            value={sensitivity}
            onChange={(e) => setSensitivity(e.target.value as OccasionSensitivity)}
            fullWidth
          >
            <MenuItem value="normal">{t('occasions.obligation.sensitivities.normal')}</MenuItem>
            <MenuItem value="private">{t('occasions.obligation.sensitivities.private')}</MenuItem>
            <MenuItem value="secret">{t('occasions.obligation.sensitivities.secret')}</MenuItem>
          </TextField>

          <TextField
            label={t('occasions.events.notes')}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            fullWidth
            multiline
            minRows={2}
          />

          {error && (
            <Typography color="error" variant="body2">
              {error}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleSave()} variant="contained" disabled={saving}>
          {t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
