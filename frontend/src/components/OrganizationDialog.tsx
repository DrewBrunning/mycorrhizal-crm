import {
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { CardOrganization, CardTemporalRange } from '../api/contacts';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import AppDialog from './AppDialog';

export interface OrganizationFormData {
  name: string;
  department?: string;
  startYear?: number;
  endYear?: number;
}

interface OrganizationDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: OrganizationFormData) => Promise<void>;
  organization?: CardOrganization | null;
  period?: CardTemporalRange;
}

function parseYear(value: string): number | undefined | null {
  if (value.trim() === '') return undefined;
  const year = Number.parseInt(value, 10);
  return Number.isNaN(year) ? null : year;
}

export default function OrganizationDialog({
  open,
  onClose,
  onSave,
  organization,
  period,
}: OrganizationDialogProps) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [department, setDepartment] = useState('');
  const [startYear, setStartYear] = useState('');
  const [endYear, setEndYear] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (open) {
      setName(organization?.name || '');
      setDepartment(organization?.units?.[0]?.name || '');
      setStartYear(period?.start?.year != null ? String(period.start.year) : '');
      setEndYear(period?.end?.year != null ? String(period.end.year) : '');
      setError('');
    }
  }, [open, organization, period]);

  const handleSave = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      setError(t('contactDetail.work.validation.nameRequired'));
      return;
    }
    const start = parseYear(startYear);
    const end = parseYear(endYear);
    if (start === null || end === null) {
      setError(t('contactDetail.work.validation.invalidPeriod'));
      return;
    }
    if (start != null && end != null && start > end) {
      setError(t('contactDetail.work.validation.invalidPeriod'));
      return;
    }

    setSaving(true);
    try {
      await onSave({
        name: trimmed,
        department: department.trim() || undefined,
        startYear: start,
        endYear: end,
      });
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
        {organization
          ? t('contactDetail.work.editOrganization')
          : t('contactDetail.work.createOrganization')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label={t('contacts.organization')}
            value={name}
            onChange={(e) => {
              markDirty();
              setName(e.target.value);
              setError('');
            }}
            fullWidth
            required
          />
          <TextField
            label={t('contacts.department')}
            value={department}
            onChange={(e) => {
              markDirty();
              setDepartment(e.target.value);
            }}
            fullWidth
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
