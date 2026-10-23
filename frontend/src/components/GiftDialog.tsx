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
import type { Activity } from '../api/activities';
import { GIFT_STATUSES, type Gift, type GiftStatus } from '../api/gifts';
import { type LifeEvent, partialDateDisplay } from '../api/lifeEvents';
import { useDateFormat } from '../DateFormatProvider';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import { isHttpUrlString, looksLikeAbsoluteUri } from '../utils/linkResolution';
import { calendarDateOnly } from '../utils/localDate';
import AppDialog from './AppDialog';
import DateField from './DateField';

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
  // The status a brand-new gift starts at (T46): each section of the gift
  // list pre-seeds this so recording something already given/received needs no
  // dropdown detour. Only used when `gift` is null — editing always keeps the
  // existing gift's own status.
  initialStatus?: GiftStatus;
  // The contact's life events / activities, offered as optional "relates to"
  // links. Empty lists just hide the selectors.
  lifeEvents: LifeEvent[];
  activities: Activity[];
}

// Converts a stored gift date back to the "YYYY-MM-DD" a date input expects.
// A gift date is a calendar day stored at 00:00Z (ADR 0015's 2026-10-23
// amendment), so its UTC date is the picked day -- reading it in the local
// zone would show the previous day west of UTC. A legacy value that is not
// at UTC midnight (written before the server normalized gift dates) falls
// back to the local date it was picked as.
function toDateInputValue(iso: string | undefined): string {
  if (!iso) return '';
  const day = calendarDateOnly(iso);
  if (day !== iso) return day;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${dd}`;
}

export default function GiftDialog({
  open,
  onClose,
  onSave,
  gift,
  initialStatus = 'idea',
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
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (open) {
      setStatus(gift ? gift.status : initialStatus);
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
  }, [open, gift, initialStatus]);

  const handleSave = async () => {
    const trimmed = description.trim();
    if (!trimmed) {
      setError(t('gifts.validation.descriptionRequired'));
      return;
    }

    // "shop.example.com/mug" is what people actually paste, and both this
    // validator and the backend's would accept it — but GiftList only links a
    // value that is an absolute URI, so it would save without complaint and
    // then render as dead text. Default a scheme-less value to https, which is
    // what the field's own placeholder already promises. An absolute URI is
    // left alone, so an unsafe scheme still reaches the check below.
    let normalizedUrl = url.trim();
    if (normalizedUrl !== '' && !looksLikeAbsoluteUri(normalizedUrl)) {
      normalizedUrl = `https://${normalizedUrl}`;
    }

    // Mirror the backend's `httpurl` validator client-side so a non-http
    // scheme is a readable message here rather than a 400 from the API.
    if (normalizedUrl !== '' && !isHttpUrlString(normalizedUrl)) {
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
      url: normalizedUrl || undefined,
      notes: notes.trim() || undefined,
      occasion: occasion.trim() || undefined,
      // The picked calendar day at 00:00Z, like an activity date.
      date: date ? `${date}T00:00:00Z` : null,
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
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>{gift ? t('gifts.editTitle') : t('gifts.createTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            select
            label={t('gifts.statusLabel')}
            value={status}
            onChange={(e) => {
              markDirty();
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
              markDirty();
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
              markDirty();
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
            onChange={(e) => {
              markDirty();
              setNotes(e.target.value);
              setError('');
            }}
            fullWidth
            multiline
            minRows={2}
            helperText={t('gifts.notesHint')}
          />

          <TextField
            label={t('gifts.occasion')}
            value={occasion}
            onChange={(e) => {
              markDirty();
              setOccasion(e.target.value);
            }}
            fullWidth
            helperText={t('gifts.occasionHint')}
          />

          <DateField
            label={t('gifts.date')}
            value={date}
            onChange={(value) => {
              markDirty();
              setDate(value);
            }}
          />

          <Box
            sx={{
              display: 'flex',
              gap: 1,
            }}
          >
            <TextField
              label={t('gifts.amount')}
              type="number"
              value={amount}
              onChange={(e) => {
                markDirty();
                setAmount(e.target.value);
              }}
              fullWidth
              slotProps={{
                inputLabel: { shrink: true },
                htmlInput: { min: 0, step: '0.01' },
              }}
            />
            <TextField
              label={t('gifts.currency')}
              value={currency}
              onChange={(e) => {
                markDirty();
                setCurrency(e.target.value.toUpperCase());
              }}
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
                markDirty();
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
                markDirty();
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
                  {a.date ? ` — ${formatDate(calendarDateOnly(a.date))}` : ''}
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
