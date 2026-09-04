import {
  Button,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import AppDialog from './AppDialog';

interface ConfirmDiscardDialogProps {
  open: boolean;
  onKeepEditing: () => void;
  onDiscard: () => void;
}

// Issue #557: the confirmation step every dirty-form close guard in this app
// shows before actually discarding -- Escape, the Cancel button, or (for
// SourceImportWizard) leaving the review step, all route through this
// instead of clearing state unconditionally.
export default function ConfirmDiscardDialog({
  open,
  onKeepEditing,
  onDiscard,
}: ConfirmDiscardDialogProps) {
  const { t } = useTranslation();

  return (
    <AppDialog open={open} onClose={onKeepEditing} maxWidth="xs" fullWidth>
      <DialogTitle>{t('common.unsavedChangesTitle')}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t('common.unsavedChangesMessage')}</DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onKeepEditing} autoFocus>
          {t('common.keepEditing')}
        </Button>
        <Button onClick={onDiscard} color="error">
          {t('common.discard')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
