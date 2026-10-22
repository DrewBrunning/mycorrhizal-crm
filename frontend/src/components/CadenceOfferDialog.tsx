import {
  Button,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { CadencePolicyInput } from '../api/cadencePolicies';
import AppDialog from './AppDialog';

interface CadenceOfferDialogProps {
  open: boolean;
  contactName: string;
  offer: CadencePolicyInput | null;
  onAccept: (offer: CadencePolicyInput) => Promise<void>;
  onClose: () => void;
}

// Issue #1797 (ADR-0023 amendment, maintainer decision 1): after setting a
// target closeness on a contact with no cadence, offer to create the cadence
// the target implies. Dismissing makes no request; accepting creates the
// policy and lets the caller refresh the score so interval_source flips to
// cadence_policy.
export default function CadenceOfferDialog({
  open,
  contactName,
  offer,
  onAccept,
  onClose,
}: CadenceOfferDialogProps) {
  const { t } = useTranslation();
  const [saving, setSaving] = useState(false);

  const handleAccept = async () => {
    if (!offer || saving) return;
    setSaving(true);
    try {
      await onAccept(offer);
      onClose();
    } catch (err) {
      // Leave the dialog open so the user can retry; the caller owns the
      // error message.
      console.error('Error accepting cadence offer:', err);
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{t('targetCloseness.offer.title')}</DialogTitle>
      <DialogContent>
        <DialogContentText>
          {t('targetCloseness.offer.body', {
            name: contactName,
            days: offer?.target_interval_days,
          })}
        </DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('targetCloseness.offer.dismiss')}
        </Button>
        <Button onClick={() => void handleAccept()} variant="contained" disabled={saving || !offer}>
          {t('targetCloseness.offer.accept')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
