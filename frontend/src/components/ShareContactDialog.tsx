import { useState, useEffect, useMemo } from 'react';
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
} from '@mui/material';
import AppDialog from './AppDialog';
import FieldSectionPicker from './FieldSectionPicker';
import { EXPORT_FIELD_SECTIONS } from '../api/export';
import { createContactShare, getUserDirectory, UserDirectoryEntry } from '../api/contactShares';
import { getErrorMessage } from '../utils/errorHandler';

interface ShareContactDialogProps {
  open: boolean;
  onClose: () => void;
  vcardUID: string;
  onShared?: () => void;
}

// ShareContactDialog is P1's "Share" dialog (P1) -- a one-time filtered copy sent to another user
// on the same instance. Reuses T9's FieldSectionPicker exactly (the same
// sensitivity foot-gun guard ExportFieldPickerDialog uses), swapping the
// format radio group for a recipient picker since JSContact is the only wire
// format a share round-trips through.
export default function ShareContactDialog({ open, onClose, vcardUID, onShared }: ShareContactDialogProps) {
  const { t } = useTranslation();
  const [recipients, setRecipients] = useState<UserDirectoryEntry[]>([]);
  const [toUserID, setToUserID] = useState<number | ''>('');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sensitiveRevealed, setSensitiveRevealed] = useState(false);
  const [sharing, setSharing] = useState(false);
  const [error, setError] = useState('');

  const defaultSelection = useMemo(
    () => new Set(EXPORT_FIELD_SECTIONS.filter((s) => !s.sensitive).map((s) => s.token)),
    []
  );

  useEffect(() => {
    if (!open) return;
    setSelected(new Set(defaultSelection));
    setSensitiveRevealed(false);
    setToUserID('');
    setError('');
    getUserDirectory()
      .then(setRecipients)
      .catch((err) => setError(getErrorMessage(err)));
  }, [open, defaultSelection]);

  const toggle = (token: string, checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(token);
      } else {
        next.delete(token);
      }
      return next;
    });
  };

  const handleShare = async () => {
    if (toUserID === '') return;
    const includeSensitive =
      sensitiveRevealed && EXPORT_FIELD_SECTIONS.some((s) => s.sensitive && selected.has(s.token));

    setSharing(true);
    setError('');
    try {
      await createContactShare({
        to_user_id: toUserID,
        vcard_uid: vcardUID,
        sections: [...selected],
        include_sensitive: includeSensitive,
      });
      onShared?.();
      onClose();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSharing(false);
    }
  };

  const handleClose = () => {
    if (sharing) return;
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('contactShares.shareDialog.title')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2" color="text.secondary">
            {t('contactShares.shareDialog.description')}
          </Typography>

          <TextField
            select
            label={t('contactShares.shareDialog.recipient')}
            value={toUserID}
            onChange={(e) => setToUserID(e.target.value === '' ? '' : Number(e.target.value))}
            fullWidth
          >
            <MenuItem value="">
              <em>{t('contactShares.shareDialog.selectRecipient')}</em>
            </MenuItem>
            {recipients.map((u) => (
              <MenuItem key={u.id} value={u.id}>
                {u.username}
              </MenuItem>
            ))}
          </TextField>

          <FieldSectionPicker
            selected={selected}
            onToggle={toggle}
            sensitiveRevealed={sensitiveRevealed}
            onReveal={() => setSensitiveRevealed(true)}
          />

          {error && <Alert severity="error" sx={{ py: 0 }}>{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={sharing}>
          {t('common.cancel')}
        </Button>
        <Button onClick={handleShare} variant="contained" disabled={sharing || toUserID === '' || selected.size === 0}>
          {sharing ? t('contactShares.shareDialog.sharing') : t('contactShares.shareDialog.shareButton')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
