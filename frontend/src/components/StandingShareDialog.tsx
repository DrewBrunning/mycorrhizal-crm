import { DialogTitle } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ShareGrantEntityType, ShareGrantOwnerView } from '../api/shareGrants';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import AppDialog from './AppDialog';
import StandingShareForm from './StandingShareForm';

interface StandingShareDialogProps {
  open: boolean;
  onClose: () => void;
  // Contacts use ShareContactDialog (which also offers the one-time copy).
  entityType: Exclude<ShareGrantEntityType, 'contact'>;
  entityId: string;
  onShared?: (grant: ShareGrantOwnerView) => void;
}

// StandingShareDialog is the Share dialog for a note, activity or circle: only a
// standing share exists for these (the one-time copy is contact-only, ADR 0042).
export default function StandingShareDialog({
  open,
  onClose,
  entityType,
  entityId,
  onShared,
}: StandingShareDialogProps) {
  const { t } = useTranslation();
  const { markDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);
  return (
    <AppDialog open={open} onClose={requestClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t(`standingShares.dialogTitle.${entityType}`)}</DialogTitle>
      <StandingShareForm
        entityType={entityType}
        entityId={entityId}
        onDirty={markDirty}
        onCancel={requestClose}
        onCreated={(grant) => {
          onShared?.(grant);
          onClose();
        }}
      />
      {guardElement}
    </AppDialog>
  );
}
