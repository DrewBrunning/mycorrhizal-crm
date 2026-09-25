import {
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Switch,
  TextField,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { DataDecayPolicy, DataDecayPolicyInput } from '../api/dataDecayPolicies';
import { useSnackbar } from '../context/SnackbarContext';
import { getErrorMessage, handleError } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface DataDecayDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (input: DataDecayPolicyInput) => Promise<void>;
  entityId: string; // Contact.VCardUID
  policy?: DataDecayPolicy | null; // undefined/null = create mode
}

export default function DataDecayDialog({
  open,
  onClose,
  onSave,
  entityId,
  policy,
}: DataDecayDialogProps) {
  const { t } = useTranslation();
  const { showError } = useSnackbar();
  const [intervalDays, setIntervalDays] = useState<number>(365);
  const [active, setActive] = useState(true);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  const isEditing = !!policy;

  useEffect(() => {
    if (open) {
      setIntervalDays(policy?.interval_days ?? 365);
      setActive(policy?.active ?? true);
      setError('');
    }
  }, [open, policy]);

  const handleClose = () => {
    if (saving) return;
    setError('');
    onClose();
  };

  const handleSave = async () => {
    if (!Number.isFinite(intervalDays) || intervalDays < 1) {
      setError(t('dataDecay.intervalRequired'));
      return;
    }
    setSaving(true);
    try {
      await onSave({
        entity_id: entityId,
        interval_days: intervalDays,
        active,
      });
      handleClose();
    } catch (err) {
      handleError(err, { operation: 'saving data decay policy' }, { showError });
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{isEditing ? t('dataDecay.editTitle') : t('dataDecay.createTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label={t('dataDecay.intervalDays')}
            type="number"
            value={intervalDays}
            onChange={(e) => {
              setIntervalDays(Number(e.target.value));
              setError('');
            }}
            fullWidth
            required
            error={!!error && intervalDays < 1}
            helperText={t('dataDecay.intervalHint')}
            slotProps={{
              htmlInput: { min: 1 },
            }}
          />
          <FormControlLabel
            control={<Switch checked={active} onChange={(e) => setActive(e.target.checked)} />}
            label={t('dataDecay.activeLabel')}
          />
          {error && <Box sx={{ color: 'error.main', fontSize: '0.875rem' }}>{error}</Box>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleSave()} variant="contained" disabled={saving}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
