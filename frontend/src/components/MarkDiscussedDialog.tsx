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
import type { ConversationAgenda } from '../api/conversationAgenda';
import { useDateFormat } from '../DateFormatProvider';
import AppDialog from './AppDialog';

interface MarkDiscussedDialogProps {
  open: boolean;
  onClose: () => void;
  onConfirm: (activityId?: number) => Promise<void>;
  item?: ConversationAgenda | null;
  // The contact's interactions, offered as an optional "recorded in" link.
  // An empty list just hides the selector — marking discussed without a link
  // is always valid.
  activities: Activity[];
}

export default function MarkDiscussedDialog({
  open,
  onClose,
  onConfirm,
  item,
  activities,
}: MarkDiscussedDialogProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [activityId, setActivityId] = useState<number | ''>('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setActivityId('');
      setError('');
    }
  }, [open]);

  const handleConfirm = async () => {
    setSaving(true);
    try {
      await onConfirm(activityId === '' ? undefined : activityId);
      onClose();
    } catch {
      setError(t('conversationAgenda.validation.discussFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('conversationAgenda.discussTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
            {item?.content}
          </Typography>
          {activities.length > 0 && (
            <TextField
              select
              label={t('conversationAgenda.discussActivity')}
              value={activityId}
              onChange={(e) => {
                setActivityId(e.target.value === '' ? '' : Number(e.target.value));
                setError('');
              }}
              fullWidth
              helperText={t('conversationAgenda.discussActivityHint')}
            >
              <MenuItem value="">{t('conversationAgenda.noActivity')}</MenuItem>
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
          {t('conversationAgenda.cancel')}
        </Button>
        <Button onClick={handleConfirm} variant="contained" disabled={saving}>
          {t('conversationAgenda.discussConfirm')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
