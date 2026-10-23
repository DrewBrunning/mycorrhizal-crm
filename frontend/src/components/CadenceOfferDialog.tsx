import {
  Alert,
  Button,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { CadencePolicyInput } from '../api/cadencePolicies';
import { getErrorMessage } from '../utils/errorHandler';
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
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    setError(null);
    onClose();
  };

  const handleAccept = async () => {
    if (!offer || saving) return;
    setSaving(true);
    setError(null);
    try {
      await onAccept(offer);
      close();
    } catch (err) {
      // Leave the dialog open so the user can retry, and say why: the
      // cadence save (useCadencePolicy.handleSave) reports nothing itself,
      // so a failure here used to reach only the console.
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={close} maxWidth="xs" fullWidth>
      <DialogTitle>{t('targetCloseness.offer.title')}</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <DialogContentText>
          {t('targetCloseness.offer.body', {
            name: contactName,
            days: offer?.target_interval_days,
          })}
        </DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={close} disabled={saving}>
          {t('targetCloseness.offer.dismiss')}
        </Button>
        <Button onClick={() => void handleAccept()} variant="contained" disabled={saving || !offer}>
          {t('targetCloseness.offer.accept')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
