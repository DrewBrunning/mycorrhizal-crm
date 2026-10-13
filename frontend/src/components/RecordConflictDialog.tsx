import {
  Button,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import AppDialog from './AppDialog';

interface RecordConflictDialogProps {
  open: boolean;
  // Dismiss the dialog but leave the user's edit open (so they can copy it).
  onKeepEditing: () => void;
  // Discard the edit and refetch the latest server version.
  onReload: () => void;
}

// ADR 0008 / ADR 0010: shown when a save is answered with 412 because the
// record changed elsewhere since the edit began. There is deliberately no
// automatic merge -- the user either loads the latest version (discarding
// their edit) or keeps the edit open to copy from it.
export default function RecordConflictDialog({
  open,
  onKeepEditing,
  onReload,
}: RecordConflictDialogProps) {
  const { t } = useTranslation();

  return (
    <AppDialog open={open} onClose={onKeepEditing} maxWidth="xs" fullWidth>
      <DialogTitle>{t('common.conflictTitle')}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t('common.conflictMessage')}</DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onKeepEditing}>{t('common.keepEditing')}</Button>
        <Button onClick={onReload} variant="contained" autoFocus>
          {t('common.conflictReload')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
