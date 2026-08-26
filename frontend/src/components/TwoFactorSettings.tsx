import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import VerifiedUserIcon from '@mui/icons-material/VerifiedUser';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { QRCodeSVG } from 'qrcode.react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  confirmTwoFactor,
  disableTwoFactor,
  getTwoFactorStatus,
  regenerateRecoveryCodes,
  setupTwoFactor,
} from '../api/users';
import { useSnackbar } from '../context/SnackbarContext';
import AppDialog from './AppDialog';

// TwoFactorSettings is the settings-page card for N8 (issue #158): enroll,
// disable, and regenerate recovery codes for TOTP two-factor auth.
//
// Enrollment is a small wizard: setup mints a secret (shown as QR + manual
// key), the user enters a live code to confirm, and the recovery codes are
// shown exactly once.
export default function TwoFactorSettings() {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();

  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(true);

  // Enrollment wizard state.
  const [setupOpen, setSetupOpen] = useState(false);
  const [setupSecret, setSetupSecret] = useState('');
  const [setupUrl, setSetupUrl] = useState('');
  const [confirmCode, setConfirmCode] = useState('');
  const [enrolling, setEnrolling] = useState(false);
  const [setupError, setSetupError] = useState('');
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [copied, setCopied] = useState(false);

  // Code-prompt dialog (disable / regenerate).
  const [action, setAction] = useState<'disable' | 'regenerate' | null>(null);
  const [actionCode, setActionCode] = useState('');
  const [actionBusy, setActionBusy] = useState(false);
  const [actionError, setActionError] = useState('');

  const refreshStatus = async () => {
    try {
      const status = await getTwoFactorStatus();
      setEnabled(status.enabled);
    } catch {
      // Non-critical on the settings page; keep whatever we have.
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refreshStatus();
  }, []);

  const handleStartSetup = async () => {
    setEnrolling(true);
    setSetupError('');
    try {
      const result = await setupTwoFactor();
      setSetupSecret(result.secret);
      setSetupUrl(result.otpauth_url);
      setConfirmCode('');
      setSetupOpen(true);
    } catch (err) {
      setSetupError(err instanceof Error ? err.message : t('settings.twoFactor.setupError'));
    } finally {
      setEnrolling(false);
    }
  };

  const handleConfirm = async (e: React.FormEvent) => {
    e.preventDefault();
    setEnrolling(true);
    setSetupError('');
    try {
      const result = await confirmTwoFactor(confirmCode.trim());
      setSetupOpen(false);
      setSetupSecret('');
      setSetupUrl('');
      setConfirmCode('');
      setRecoveryCodes(result.recovery_codes);
      setCopied(false);
      setEnabled(true);
      showSuccess(t('settings.twoFactor.enableSuccess'));
    } catch (err) {
      setSetupError(err instanceof Error ? err.message : t('settings.twoFactor.invalidCode'));
    } finally {
      setEnrolling(false);
    }
  };

  const handleCopyCodes = () => {
    if (recoveryCodes) {
      navigator.clipboard.writeText(recoveryCodes.join('\n'));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const openAction = (which: 'disable' | 'regenerate') => {
    setAction(which);
    setActionCode('');
    setActionError('');
  };

  const submitAction = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!action) return;
    setActionBusy(true);
    setActionError('');
    try {
      if (action === 'disable') {
        await disableTwoFactor(actionCode.trim());
        setEnabled(false);
        showSuccess(t('settings.twoFactor.disableSuccess'));
      } else {
        const result = await regenerateRecoveryCodes(actionCode.trim());
        setRecoveryCodes(result.recovery_codes);
        setCopied(false);
        showSuccess(t('settings.twoFactor.regenerateSuccess'));
      }
      setAction(null);
      setActionCode('');
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('settings.twoFactor.invalidCode'));
    } finally {
      setActionBusy(false);
    }
  };

  const closeRecoveryDialog = () => {
    setRecoveryCodes(null);
    setCopied(false);
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <VerifiedUserIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('settings.twoFactor.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {loading ? (
          <CircularProgress size={24} />
        ) : enabled === false ? (
          <Stack spacing={1}>
            <Typography variant="body2" color="text.secondary">
              {t('settings.twoFactor.description')}
            </Typography>
            <Box>
              <Button
                variant="contained"
                size="small"
                onClick={handleStartSetup}
                disabled={enrolling}
              >
                {enrolling
                  ? t('settings.twoFactor.settingUp')
                  : t('settings.twoFactor.enableButton')}
              </Button>
            </Box>
            {/* Only shown when the setup dialog is closed — while it is open the
                same error renders inside the dialog. */}
            {setupError && !setupOpen && (
              <Alert severity="error" sx={{ py: 0 }}>
                {setupError}
              </Alert>
            )}
          </Stack>
        ) : (
          <Stack spacing={1}>
            <Alert severity="success" sx={{ py: 0 }}>
              {t('settings.twoFactor.enabledBadge')}
            </Alert>
            <Typography variant="body2" color="text.secondary">
              {t('settings.twoFactor.enabledDescription')}
            </Typography>
            <Stack direction="row" spacing={1}>
              <Button size="small" variant="outlined" onClick={() => openAction('regenerate')}>
                {t('settings.twoFactor.regenerateButton')}
              </Button>
              <Button
                size="small"
                variant="outlined"
                color="error"
                onClick={() => openAction('disable')}
              >
                {t('settings.twoFactor.disableButton')}
              </Button>
            </Stack>
          </Stack>
        )}
      </CardContent>

      {/* Enrollment wizard: QR + manual key + confirm code */}
      <AppDialog
        open={setupOpen}
        onClose={() => !enrolling && setSetupOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>{t('settings.twoFactor.setup.title')}</DialogTitle>
        <DialogContent>
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              {t('settings.twoFactor.setup.description')}
            </Typography>
            {setupUrl && (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 1 }}>
                <QRCodeSVG value={setupUrl} size={180} />
              </Box>
            )}
            <TextField
              label={t('settings.twoFactor.setup.manualKey')}
              value={setupSecret}
              InputProps={{ readOnly: true }}
              size="small"
              fullWidth
              inputProps={{ style: { fontFamily: 'monospace' } }}
            />
            <form onSubmit={handleConfirm}>
              <Stack spacing={1.5}>
                <TextField
                  label={t('settings.twoFactor.setup.codeLabel')}
                  type="text"
                  inputMode="numeric"
                  value={confirmCode}
                  onChange={(e) => setConfirmCode(e.target.value)}
                  required
                  fullWidth
                  size="small"
                  autoFocus
                  helperText={t('settings.twoFactor.setup.codeHelp')}
                />
                {setupError && (
                  <Alert severity="error" sx={{ py: 0 }}>
                    {setupError}
                  </Alert>
                )}
                <Button
                  type="submit"
                  variant="contained"
                  disabled={enrolling || confirmCode.trim().length === 0}
                >
                  {enrolling
                    ? t('settings.twoFactor.setup.enabling')
                    : t('settings.twoFactor.setup.confirmButton')}
                </Button>
              </Stack>
            </form>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSetupOpen(false)} disabled={enrolling}>
            {t('common.cancel')}
          </Button>
        </DialogActions>
      </AppDialog>

      {/* Code prompt for disable / regenerate */}
      <AppDialog
        open={action !== null}
        onClose={() => !actionBusy && setAction(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>
          {action === 'disable'
            ? t('settings.twoFactor.disable.title')
            : t('settings.twoFactor.regenerate.title')}
        </DialogTitle>
        <DialogContent>
          <form onSubmit={submitAction}>
            <Stack spacing={1.5}>
              <Typography variant="body2" color="text.secondary">
                {action === 'disable'
                  ? t('settings.twoFactor.disable.description')
                  : t('settings.twoFactor.regenerate.description')}
              </Typography>
              <TextField
                label={t('settings.twoFactor.codePromptLabel')}
                type="text"
                value={actionCode}
                onChange={(e) => setActionCode(e.target.value)}
                required
                fullWidth
                size="small"
                autoFocus
              />
              {actionError && (
                <Alert severity="error" sx={{ py: 0 }}>
                  {actionError}
                </Alert>
              )}
              <Button
                type="submit"
                variant="contained"
                color={action === 'disable' ? 'error' : 'primary'}
                disabled={actionBusy}
              >
                {actionBusy ? t('settings.twoFactor.submitting') : t('settings.twoFactor.confirm')}
              </Button>
            </Stack>
          </form>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAction(null)} disabled={actionBusy}>
            {t('common.cancel')}
          </Button>
        </DialogActions>
      </AppDialog>

      {/* Recovery codes — shown exactly once */}
      <Dialog open={recoveryCodes !== null} onClose={closeRecoveryDialog} maxWidth="sm" fullWidth>
        <DialogTitle>{t('settings.twoFactor.recovery.title')}</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            {t('settings.twoFactor.recovery.warning')}
          </Alert>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            {t('settings.twoFactor.recovery.description')}
          </Typography>
          <Box
            sx={{
              fontFamily: 'monospace',
              fontSize: '0.9rem',
              border: 1,
              borderColor: 'divider',
              borderRadius: 1,
              p: 1.5,
              display: 'grid',
              gridTemplateColumns: '1fr 1fr',
              gap: 0.5,
            }}
          >
            {recoveryCodes?.map((code) => (
              <Typography key={code} variant="body2" sx={{ fontFamily: 'monospace' }}>
                {code}
              </Typography>
            ))}
          </Box>
          <Box sx={{ mt: 1 }}>
            <Button size="small" startIcon={<ContentCopyIcon />} onClick={handleCopyCodes}>
              {copied
                ? t('settings.twoFactor.recovery.copied')
                : t('settings.twoFactor.recovery.copy')}
            </Button>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button variant="contained" onClick={closeRecoveryDialog}>
            {t('settings.twoFactor.recovery.done')}
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
}
