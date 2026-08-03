import { useState, useEffect } from 'react';
import { DialogTitle, DialogContent, DialogActions, TextField, Button, Box, FormGroup, FormControlLabel, Checkbox, Typography } from '@mui/material';
import AppDialog from './AppDialog';
import { useTranslation } from 'react-i18next';
import { CadencePolicy, CadencePolicyInput, QUALIFYING_INTERACTION_TYPE_TOKENS } from '../api/cadencePolicies';
import { useSnackbar } from '../context/SnackbarContext';
import { handleError, getErrorMessage } from '../utils/errorHandler';

interface CadenceDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (input: CadencePolicyInput) => Promise<void>;
  entityId: string; // Contact.VCardUID
  policy?: CadencePolicy | null; // undefined/null = create mode
}

export default function CadenceDialog({ open, onClose, onSave, entityId, policy }: CadenceDialogProps) {
  const { t } = useTranslation();
  const { showError } = useSnackbar();
  const [intervalDays, setIntervalDays] = useState<number>(30);
  const [qualifyingTypes, setQualifyingTypes] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  const isEditing = !!policy;

  useEffect(() => {
    if (open) {
      setIntervalDays(policy?.target_interval_days ?? 30);
      setQualifyingTypes(policy?.qualifying_types ?? []);
      setError('');
    }
  }, [open, policy]);

  const toggleType = (token: string) => {
    setQualifyingTypes((prev) =>
      prev.includes(token) ? prev.filter((t) => t !== token) : [...prev, token]
    );
  };

  const handleClose = () => {
    if (saving) return;
    setError('');
    onClose();
  };

  const handleSave = async () => {
    if (!Number.isFinite(intervalDays) || intervalDays < 1) {
      setError(t('cadence.intervalRequired'));
      return;
    }
    setSaving(true);
    try {
      await onSave({ entity_id: entityId, target_interval_days: intervalDays, qualifying_types: qualifyingTypes });
      handleClose();
    } catch (err) {
      handleError(err, { operation: 'saving cadence' }, { showError });
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {isEditing ? t('cadence.editTitle') : t('cadence.createTitle')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label={t('cadence.intervalDays')}
            type="number"
            inputProps={{ min: 1 }}
            value={intervalDays}
            onChange={(e) => {
              setIntervalDays(Number(e.target.value));
              setError('');
            }}
            fullWidth
            required
            error={!!error && intervalDays < 1}
            helperText={t('cadence.intervalHint')}
          />
          <Box>
            <Typography variant="subtitle2" gutterBottom>
              {t('cadence.qualifyingTypes')}
            </Typography>
            <Typography variant="caption" color="text.secondary" gutterBottom>
              {t('cadence.qualifyingHint')}
            </Typography>
            <FormGroup>
              {QUALIFYING_INTERACTION_TYPE_TOKENS.map((token) => (
                <FormControlLabel
                  key={token}
                  control={
                    <Checkbox
                      checked={qualifyingTypes.includes(token)}
                      onChange={() => toggleType(token)}
                    />
                  }
                  label={t(`cadence.types.${token}`)}
                />
              ))}
            </FormGroup>
          </Box>
          {error && (
            <Box sx={{ color: 'error.main', fontSize: '0.875rem' }}>{error}</Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
