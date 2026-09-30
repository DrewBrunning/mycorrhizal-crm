import {
  Alert,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import AppDialog from './AppDialog';

interface SecondFactorProofDialogProps {
  open: boolean;
  busy: boolean;
  error: string;
  // Offer "verify with a passkey instead" (the account has one and the browser
  // supports the ceremony).
  canUsePasskey: boolean;
  onSubmitCode: (code: string) => void;
  onUsePasskey: () => void;
  onClose: () => void;
  // Override the enrollment wording for another proof-gated action (issue #1354,
  // recovery-code regeneration).
  title?: string;
  description?: string;
}

// SecondFactorProofDialog asks for a live proof before an ADDITIONAL second
// factor is enrolled (issue #1337): a TOTP/recovery code, or an assertion from
// an existing passkey. Removal has the same bar and its own dialog
// (PasskeySettings' remove flow); the first factor is enrolled without one.
export default function SecondFactorProofDialog({
  open,
  busy,
  error,
  canUsePasskey,
  onSubmitCode,
  onUsePasskey,
  onClose,
  title,
  description,
}: SecondFactorProofDialogProps) {
  const { t } = useTranslation();
  const [code, setCode] = useState('');

  useEffect(() => {
    if (open) setCode('');
  }, [open]);

  return (
    <AppDialog open={open} onClose={() => !busy && onClose()} maxWidth="xs" fullWidth>
      <DialogTitle>{title ?? t('settings.secondFactorProof.title')}</DialogTitle>
      <DialogContent>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            onSubmitCode(code.trim());
          }}
        >
          <Stack spacing={1.5}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {description ?? t('settings.secondFactorProof.description')}
            </Typography>
            <TextField
              label={t('settings.secondFactorProof.codeLabel')}
              type="text"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              required
              fullWidth
              size="small"
              autoFocus
            />
            {error && (
              <Alert severity="error" sx={{ py: 0 }}>
                {error}
              </Alert>
            )}
            <Button type="submit" variant="contained" disabled={busy || code.trim().length === 0}>
              {busy
                ? t('settings.secondFactorProof.submitting')
                : t('settings.secondFactorProof.confirm')}
            </Button>
            {canUsePasskey && (
              <Button variant="outlined" onClick={onUsePasskey} disabled={busy}>
                {t('settings.secondFactorProof.usePasskey')}
              </Button>
            )}
          </Stack>
        </form>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={busy}>
          {t('common.cancel')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
