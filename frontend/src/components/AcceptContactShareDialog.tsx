import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  TextField,
  MenuItem,
  Alert,
  Chip,
  CircularProgress,
} from '@mui/material';
import AppDialog from './AppDialog';
import { ContactShare } from '../api/contactShares';
import { ImportPreviewResponse, RowImportAction } from '../api/import';
import { getErrorMessage } from '../utils/errorHandler';

type RowAction = 'add' | 'update' | 'skip';

interface AcceptContactShareDialogProps {
  open: boolean;
  onClose: () => void;
  share: ContactShare;
  onAcceptPreview: (shareId: string) => Promise<ImportPreviewResponse | undefined>;
  onConfirm: (shareId: string, sessionId: string, actions: RowImportAction[]) => Promise<unknown>;
}

// AcceptContactShareDialog is P1's single-row confirm step (docs/fork-plan/
// tickets/31-P1-contact-sharing.md) -- a scoped-down version of
// ImportContactsDialog's preview table (duplicate-match chip + add/update/
// skip picker), since a share is always exactly one contact. This is the
// "ask-the-user" merge-policy decision made concrete: the recipient
// explicitly picks add (create new) or update (merge via the existing,
// already-tested MergeImportedContact policy) -- never automatic.
export default function AcceptContactShareDialog({
  open,
  onClose,
  share,
  onAcceptPreview,
  onConfirm,
}: AcceptContactShareDialogProps) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<ImportPreviewResponse | null>(null);
  const [action, setAction] = useState<RowAction>('add');
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState('');

  const loadPreview = useCallback(() => {
    setPreview(null);
    setError('');
    setLoading(true);
    onAcceptPreview(share.id)
      .then((resp) => {
        if (!resp) return;
        setPreview(resp);
        const suggested = resp.rows[0]?.suggested_action;
        setAction(suggested === 'update' || suggested === 'skip' ? suggested : 'add');
      })
      .catch((err) => setError(getErrorMessage(err)))
      .finally(() => setLoading(false));
  }, [onAcceptPreview, share.id]);

  useEffect(() => {
    if (open) loadPreview();
  }, [open, loadPreview]);

  const row = preview?.rows[0];

  const handleConfirmClick = async () => {
    if (!preview || !row) return;
    setConfirming(true);
    setError('');
    try {
      await onConfirm(share.id, preview.session_id, [{ row_index: row.row_index, action }]);
      onClose();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setConfirming(false);
    }
  };

  const handleClose = () => {
    if (confirming) return;
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('contactShares.acceptDialog.title')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2" color="text.secondary">
            {t('contactShares.acceptDialog.description')}
          </Typography>

          {loading && (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
              <CircularProgress size={24} />
            </Box>
          )}

          {row && (
            <>
              {row.duplicate_match && (
                <Chip
                  label={t('contactShares.acceptDialog.duplicateOf', {
                    name: `${row.duplicate_match.existing_firstname} ${row.duplicate_match.existing_lastname}`,
                    reason: row.duplicate_match.match_reason,
                  })}
                  color="warning"
                  variant="outlined"
                  sx={{ alignSelf: 'flex-start' }}
                />
              )}

              <TextField
                select
                label={t('contactShares.acceptDialog.actionLabel')}
                value={action}
                onChange={(e) => setAction(e.target.value as RowAction)}
                fullWidth
              >
                <MenuItem value="add">{t('contactShares.acceptDialog.actionAdd')}</MenuItem>
                {row.duplicate_match && (
                  <MenuItem value="update">{t('contactShares.acceptDialog.actionUpdate')}</MenuItem>
                )}
                <MenuItem value="skip">{t('contactShares.acceptDialog.actionSkip')}</MenuItem>
              </TextField>
            </>
          )}

          {error && <Alert severity="error" sx={{ py: 0 }}>{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={confirming}>
          {t('common.cancel')}
        </Button>
        <Button onClick={handleConfirmClick} variant="contained" disabled={confirming || !row}>
          {confirming ? t('contactShares.acceptDialog.confirming') : t('contactShares.acceptDialog.confirm')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
