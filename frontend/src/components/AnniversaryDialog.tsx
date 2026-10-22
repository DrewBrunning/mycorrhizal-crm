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
import { type CardAnniversary, formatAnniversaryDate, parseAnniversaryDate } from '../api/contacts';
import AppDialog from './AppDialog';

export interface AnniversaryFormData {
  kind: 'death' | 'wedding';
  date: string;
}

interface AnniversaryDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: AnniversaryFormData) => Promise<void>;
  anniversary?: CardAnniversary | null;
}

// Date input accepts the app's two anniversary shapes: a full YYYY-MM-DD or
// a year-less --MM-DD. Anything else is rejected here rather than passed
// through as an opaque timestamp the list cannot render.
function isValidAnniversaryInput(value: string): boolean {
  return /^--\d{2}-\d{2}$/.test(value) || /^\d{4}-\d{2}-\d{2}$/.test(value);
}

export default function AnniversaryDialog({
  open,
  onClose,
  onSave,
  anniversary,
}: AnniversaryDialogProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<'death' | 'wedding'>('wedding');
  const [date, setDate] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setKind(anniversary?.kind === 'death' ? 'death' : 'wedding');
      setDate(anniversary ? (formatAnniversaryDate(anniversary.date) ?? '') : '');
      setError('');
    }
  }, [open, anniversary]);

  const handleSave = async () => {
    const trimmed = date.trim();
    if (!trimmed) {
      setError(t('contactDetail.anniversaries.validation.dateRequired'));
      return;
    }
    if (!isValidAnniversaryInput(trimmed)) {
      setError(t('contactDetail.anniversaries.validation.invalidDate'));
      return;
    }

    setSaving(true);
    try {
      await onSave({ kind, date: trimmed });
      onClose();
    } catch {
      setError(t('contactDetail.anniversaries.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {anniversary
          ? t('contactDetail.anniversaries.editTitle')
          : t('contactDetail.anniversaries.createTitle')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            select
            label={t('contacts.anniversaryFields.kindLabel')}
            value={kind}
            onChange={(e) => setKind(e.target.value as 'death' | 'wedding')}
            fullWidth
          >
            <MenuItem value="wedding">
              {t('contacts.anniversaryFields.kindOptions.wedding')}
            </MenuItem>
            <MenuItem value="death">{t('contacts.anniversaryFields.kindOptions.death')}</MenuItem>
          </TextField>
          <TextField
            label={t('contacts.anniversaryFields.date')}
            value={date}
            onChange={(e) => {
              setDate(e.target.value);
              setError('');
            }}
            placeholder="--MM-DD"
            helperText={t('contactDetail.anniversaries.dateHint')}
            fullWidth
            required
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

// Exposed for the page's save handler: converts the validated form string to
// the neutral date shape while preserving any fields the entry already had.
export function anniversaryFromForm(
  existing: CardAnniversary | null | undefined,
  data: AnniversaryFormData,
): CardAnniversary {
  return {
    ...(existing ?? {}),
    kind: data.kind,
    date: parseAnniversaryDate(data.date),
  };
}
