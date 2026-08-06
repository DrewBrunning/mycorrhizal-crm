import { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Box,
  MenuItem,
  Typography,
} from '@mui/material';
import AppDialog from './AppDialog';
import { useTranslation } from 'react-i18next';
import { Gift, GIFT_STATUSES, GiftStatus } from '../api/gifts';
import { LifeEvent, partialDateDisplay } from '../api/lifeEvents';
import { Activity } from '../api/activities';
import { useDateFormat } from '../DateFormatProvider';
import { isSafeUrlString } from '../utils/linkResolution';

// GiftFormData is what the dialog submits; the page adds entity_id and hands it
// to the hook as a GiftInput.
export interface GiftFormData {
  status: GiftStatus;
  description: string;
  // Optional product link and free-text context (T35).
  url?: string;
  notes?: string;
  occasion?: string;
  // ISO datetime (or null to clear); the dialog converts the date-only picker.
  date?: string | null;
  value_cents?: number;
  currency?: string;
  life_event_id?: string;
  activity_id?: number | null;
}

interface GiftDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: GiftFormData) => Promise<void>;
  gift?: Gift | null;
  // The contact's life events / activities, offered as optional "relates to"
  // links. Empty lists just hide the selectors.
  lifeEvents: LifeEvent[];
  activities: Activity[];
}

// Converts a stored ISO datetime back to the local "YYYY-MM-DD" a date input
// expects. Deliberately NOT iso.slice(0,10): the stored value is UTC (Go
// serializes time.Time in UTC), and slicing that would show the previous day
// for timezones west of UTC. This renders the date in the user's own zone, so
// what they picked is what they see.
function toDateInputValue(iso: string | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

export default function GiftDialog({
  open,
  onClose,
  onSave,
  gift,
  lifeEvents,
  activities,
}: GiftDialogProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [status, setStatus] = useState<GiftStatus | ''>('');
  const [description, setDescription] = useState('');
  const [url, setUrl] = useState('');
  const [notes, setNotes] = useState('');
  const [occasion, setOccasion] = useState('');
  const [date, setDate] = useState('');
  const [amount, setAmount] = useState('');
  const [currency, setCurrency] = useState('');
  const [lifeEventId, setLifeEventId] = useState('');
  const [activityId, setActivityId] = useState<number | ''>('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setStatus(gift?.status || 'idea');
      setDescription(gift?.description || '');
      setUrl(gift?.url || '');
      setNotes(gift?.notes || '');
      setOccasion(gift?.occasion || '');
      setDate(gift?.date ? toDateInputValue(gift.date) : '');
      setAmount(gift?.value_cents ? String(gift.value_cents / 100) : '');
      setCurrency(gift?.currency || '');
      setLifeEventId(gift?.life_event_id || '');
      setActivityId(gift?.activity_id != null ? gift.activity_id : '');
      setError('');
    }
  }, [open, gift]);

  const handleSave = async () => {
    const trimmed = description.trim();
    if (!trimmed) {
      setError(t('gifts.validation.descriptionRequired'));
      return;
    }

    // Mirror the backend's `safeurl` validator client-side so an unsafe
    // scheme is a readable message here rather than a 400 from the API.
    const trimmedUrl = url.trim();
    if (trimmedUrl !== '' && !isSafeUrlString(trimmedUrl)) {
      setError(t('gifts.validation.invalidUrl'));
      return;
    }

    let valueCents: number | undefined;
    if (amount.trim() !== '') {
      const parsed = parseFloat(amount);
      if (Number.isNaN(parsed) || parsed < 0) {
        setError(t('gifts.validation.invalidAmount'));
        return;
      }
      if (currency.trim() === '') {
        setError(t('gifts.validation.currencyRequired'));
        return;
      }
      valueCents = Math.round(parsed * 100);
    } else if (currency.trim() !== '') {
      setError(t('gifts.validation.currencyRequired'));
      return;
    }

    const data: GiftFormData = {
      status: (status || 'idea') as GiftStatus,
      description: trimmed,
      url: trimmedUrl || undefined,
      notes: notes.trim() || undefined,
      occasion: occasion.trim() || undefined,
      date: date ? new Date(`${date}T00:00:00`).toISOString() : null,
      value_cents: valueCents,
      currency: currency.trim().toUpperCase() || undefined,
      life_event_id: lifeEventId || undefined,
      activity_id: activityId === '' ? null : activityId,
    };

    setSaving(true);
    try {
      await onSave(data);
      onClose();
    } catch {
      setError(t('gifts.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{gift ? t('gifts.editTitle') : t('gifts.createTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            select
            label={t('gifts.statusLabel')}
            value={status}
            onChange={(e) => {
              setStatus(e.target.value as GiftStatus);
              setError('');
            }}
            fullWidth
          >
            {GIFT_STATUSES.map((token) => (
              <MenuItem key={token} value={token}>
                {t(`gifts.status.${token}`, token)}
              </MenuItem>
            ))}
          </TextField>

          <TextField
            label={t('gifts.description')}
            value={description}
            onChange={(e) => {
              setDescription(e.target.value);
              setError('');
            }}
            fullWidth
            multiline
            minRows={2}
            required
          />

          <TextField
            label={t('gifts.url')}
            type="url"
            value={url}
            onChange={(e) => {
              setUrl(e.target.value);
              setError('');
            }}
            fullWidth
            placeholder="https://"
            helperText={t('gifts.urlHint')}
          />

          <TextField
            label={t('gifts.notes')}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            fullWidth
            multiline
            minRows={2}
            helperText={t('gifts.notesHint')}
          />

          <TextField
            label={t('gifts.occasion')}
            value={occasion}
            onChange={(e) => setOccasion(e.target.value)}
            fullWidth
            helperText={t('gifts.occasionHint')}
          />

          <TextField
            label={t('gifts.date')}
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            fullWidth
            slotProps={{ inputLabel: { shrink: true } }}
          />

          <Box display="flex" gap={1}>
            <TextField
              label={t('gifts.amount')}
              type="number"
              inputProps={{ min: 0, step: '0.01' }}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              fullWidth
              slotProps={{ inputLabel: { shrink: true } }}
            />
            <TextField
              label={t('gifts.currency')}
              value={currency}
              onChange={(e) => setCurrency(e.target.value.toUpperCase())}
              fullWidth
              slotProps={{ inputLabel: { shrink: true }, htmlInput: { maxLength: 3 } }}
              placeholder="EUR"
            />
          </Box>

          {lifeEvents.length > 0 && (
            <TextField
              select
              label={t('gifts.lifeEvent')}
              value={lifeEventId}
              onChange={(e) => {
                setLifeEventId(e.target.value);
                setError('');
              }}
              fullWidth
              helperText={t('gifts.lifeEventHint')}
            >
              <MenuItem value="">{t('gifts.noLifeEvent')}</MenuItem>
              {lifeEvents.map((ev) => (
                <MenuItem key={ev.id} value={ev.id}>
                  {t(`lifeEvent.types.${ev.type}`, ev.type)}
                  {partialDateDisplay(ev.date) ? ` — ${partialDateDisplay(ev.date)}` : ''}
                </MenuItem>
              ))}
            </TextField>
          )}

          {activities.length > 0 && (
            <TextField
              select
              label={t('gifts.activity')}
              value={activityId}
              onChange={(e) => {
                setActivityId(e.target.value === '' ? '' : Number(e.target.value));
                setError('');
              }}
              fullWidth
              helperText={t('gifts.activityHint')}
            >
              <MenuItem value="">{t('gifts.noActivity')}</MenuItem>
              {activities.map((a) => (
                <MenuItem key={a.ID} value={a.ID}>
                  {a.title}
                  {formatDate(a.date) ? ` — ${formatDate(a.date)}` : ''}
                </MenuItem>
              ))}
            </TextField>
          )}

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
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
