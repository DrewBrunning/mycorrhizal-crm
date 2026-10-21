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
import AppDialog from './AppDialog';

interface WorkInformationDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (value: string) => Promise<void>;
  value?: string;
}

export default function WorkInformationDialog({
  open,
  onClose,
  onSave,
  value,
}: WorkInformationDialogProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setDraft(value || '');
      setError('');
    }
  }, [open, value]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(draft.trim());
      onClose();
    } catch {
      setError(t('contactDetail.work.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {value
          ? t('contactDetail.work.editWorkInformation')
          : t('contactDetail.work.addWorkInformation')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1 }}>
          <TextField
            label={t('contactDetail.workInfo')}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            fullWidth
            multiline
            minRows={3}
          />
          {error && (
            <Typography color="error" variant="body2" sx={{ mt: 1 }}>
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
