import DeleteForeverIcon from '@mui/icons-material/DeleteForever';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Divider,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type AccountDeletionCandidate,
  AccountDeletionRequiresPromotionError,
  deleteOwnAccount,
} from '../api/auth';
import { getTwoFactorStatus } from '../api/users';
import { logoutAndRedirect } from '../auth';
import AppDialog from './AppDialog';

// DeleteAccountSettings is the settings-page danger zone for issue #972:
// self-service account deletion. Mirrors TwoFactorSettings' shape (a
// dedicated card, a confirmation dialog gated on re-proof).
//
// Re-proof mirrors the codebase's convention for its most sensitive
// self-service actions: current password, plus a live TOTP code if 2FA is
// enabled. If the account is the only admin and other users exist, the
// backend responds 409 with a candidate list — the dialog stays open and
// grows a required "promote to admin" picker instead of closing.
export default function DeleteAccountSettings() {
  const { t } = useTranslation();

  const [totpEnabled, setTotpEnabled] = useState(false);
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState('');
  const [totpCode, setTotpCode] = useState('');
  const [candidates, setCandidates] = useState<AccountDeletionCandidate[] | null>(null);
  const [promoteUserId, setPromoteUserId] = useState<number | ''>('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    getTwoFactorStatus()
      .then((status) => setTotpEnabled(status.enabled))
      .catch(() => {
        // Non-critical here: worst case the TOTP field is hidden and the
        // backend's own 400 (missing totp_code) surfaces the same info.
      });
  }, []);

  const resetAndClose = () => {
    setOpen(false);
    setPassword('');
    setTotpCode('');
    setCandidates(null);
    setPromoteUserId('');
    setError('');
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await deleteOwnAccount(
        password,
        totpEnabled ? totpCode.trim() : undefined,
        promoteUserId === '' ? undefined : promoteUserId,
      );
      await logoutAndRedirect();
    } catch (err) {
      if (err instanceof AccountDeletionRequiresPromotionError) {
        setCandidates(err.candidates);
        setError('');
      } else {
        setError(err instanceof Error ? err.message : t('settings.deleteAccount.error'));
      }
    } finally {
      setBusy(false);
    }
  };

  const needsPromotion = candidates !== null;
  const canSubmit =
    password.length > 0 &&
    (!totpEnabled || totpCode.trim().length > 0) &&
    (!needsPromotion || promoteUserId !== '');

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <DeleteForeverIcon sx={{ mr: 1, color: 'error.main', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('settings.deleteAccount.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />
        <Stack spacing={1}>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('settings.deleteAccount.description')}
          </Typography>
          <Box>
            <Button variant="outlined" color="error" size="small" onClick={() => setOpen(true)}>
              {t('settings.deleteAccount.deleteButton')}
            </Button>
          </Box>
        </Stack>
      </CardContent>

      <AppDialog open={open} onClose={() => !busy && resetAndClose()} maxWidth="xs" fullWidth>
        <form onSubmit={handleSubmit}>
          <Box sx={{ p: 3 }}>
            <Typography variant="h6" component="h2" sx={{ mb: 2 }}>
              {t('settings.deleteAccount.dialogTitle')}
            </Typography>
            <Stack spacing={1.5}>
              <Alert severity="warning" sx={{ py: 0 }}>
                {t('settings.deleteAccount.dialogWarning')}
              </Alert>
              <TextField
                label={t('settings.deleteAccount.currentPasswordLabel')}
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                fullWidth
                size="small"
                autoFocus
              />
              {totpEnabled && (
                <TextField
                  label={t('settings.deleteAccount.totpCodeLabel')}
                  type="text"
                  inputMode="numeric"
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  required
                  fullWidth
                  size="small"
                  helperText={t('settings.deleteAccount.totpCodeHelp')}
                />
              )}
              {needsPromotion && (
                <>
                  <Alert severity="info" sx={{ py: 0 }}>
                    {t('settings.deleteAccount.promotionNotice')}
                  </Alert>
                  <FormControl fullWidth size="small" required>
                    <InputLabel id="promote-user-label">
                      {t('settings.deleteAccount.promoteUserLabel')}
                    </InputLabel>
                    <Select
                      labelId="promote-user-label"
                      label={t('settings.deleteAccount.promoteUserLabel')}
                      value={promoteUserId}
                      onChange={(e) => setPromoteUserId(e.target.value as number)}
                    >
                      {candidates?.map((c) => (
                        <MenuItem key={c.id} value={c.id}>
                          {c.username}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </>
              )}
              {error && (
                <Alert severity="error" sx={{ py: 0 }}>
                  {error}
                </Alert>
              )}
              <Stack direction="row" spacing={1} sx={{ justifyContent: 'flex-end', pt: 1 }}>
                <Button onClick={resetAndClose} disabled={busy}>
                  {t('common.cancel')}
                </Button>
                <Button
                  type="submit"
                  variant="contained"
                  color="error"
                  disabled={busy || !canSubmit}
                >
                  {busy
                    ? t('settings.deleteAccount.deleting')
                    : t('settings.deleteAccount.confirmButton')}
                </Button>
              </Stack>
            </Stack>
          </Box>
        </form>
      </AppDialog>
    </Card>
  );
}
