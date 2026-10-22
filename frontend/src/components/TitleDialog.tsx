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
import type { CardTemporalRange, CardTitle } from '../api/contacts';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import AppDialog from './AppDialog';

export interface TitleFormData {
  kind: 'title' | 'role';
  name: string;
  startYear?: number;
  endYear?: number;
}

interface TitleDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: TitleFormData) => Promise<void>;
  title?: CardTitle | null;
  period?: CardTemporalRange;
}

function parseYear(value: string): number | undefined | null {
  if (value.trim() === '') return undefined;
  const year = Number.parseInt(value, 10);
  return Number.isNaN(year) ? null : year;
}

export default function TitleDialog({ open, onClose, onSave, title, period }: TitleDialogProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<'title' | 'role'>('title');
  const [name, setName] = useState('');
  const [startYear, setStartYear] = useState('');
  const [endYear, setEndYear] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (open) {
      setKind(title?.kind === 'role' ? 'role' : 'title');
      setName(title?.name || '');
      setStartYear(period?.start?.year != null ? String(period.start.year) : '');
      setEndYear(period?.end?.year != null ? String(period.end.year) : '');
      setError('');
    }
  }, [open, title, period]);

  const handleSave = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      setError(t('contactDetail.work.validation.nameRequired'));
      return;
    }
    const start = parseYear(startYear);
    const end = parseYear(endYear);
    if (start === null || end === null || (start != null && end != null && start > end)) {
      setError(t('contactDetail.work.validation.invalidPeriod'));
      return;
    }

    setSaving(true);
    try {
      await onSave({ kind, name: trimmed, startYear: start, endYear: end });
      onClose();
    } catch {
      setError(t('contactDetail.work.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {title
          ? t('contactDetail.work.editTitleDialog')
          : t('contactDetail.work.createTitleDialog')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            select
            label={t('contactDetail.work.kind')}
            value={kind}
            onChange={(e) => {
              markDirty();
              setKind(e.target.value as 'title' | 'role');
            }}
            fullWidth
          >
            <MenuItem value="title">{t('contacts.jobTitle')}</MenuItem>
            <MenuItem value="role">{t('contacts.role')}</MenuItem>
          </TextField>
          <TextField
            label={t('contactDetail.work.titleName')}
            value={name}
            onChange={(e) => {
              markDirty();
              setName(e.target.value);
              setError('');
            }}
            fullWidth
            required
          />
          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              type="number"
              label={t('contactDetail.period.from')}
              value={startYear}
              onChange={(e) => {
                markDirty();
                setStartYear(e.target.value);
                setError('');
              }}
              slotProps={{ htmlInput: { min: 1900, max: 2100 } }}
              fullWidth
            />
            <TextField
              type="number"
              label={t('contactDetail.period.to')}
              value={endYear}
              onChange={(e) => {
                markDirty();
                setEndYear(e.target.value);
                setError('');
              }}
              slotProps={{ htmlInput: { min: 1900, max: 2100 } }}
              fullWidth
            />
          </Box>
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
