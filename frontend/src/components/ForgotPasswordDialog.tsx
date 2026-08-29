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
import { type FormEvent, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { confirmPasswordReset, requestPasswordReset } from '../api/auth';
import { useErrorAlertFocus } from '../hooks/useErrorAlertFocus';
import AppDialog from './AppDialog';

type ForgotPasswordDialogProps = {
  open: boolean;
  onClose: () => void;
};

type ResetStep = 'request' | 'confirm' | 'done';

export default function ForgotPasswordDialog({ open, onClose }: ForgotPasswordDialogProps) {
  const { t } = useTranslation();
  const [step, setStep] = useState<ResetStep>('request');
  const [email, setEmail] = useState('');
  const [token, setToken] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [loading, setLoading] = useState(false);
  // #192: same fix as LoginPage.tsx -- move focus to the error and
  // associate it with the fields instead of dropping focus to <body>. Both
  // steps share this hook's ref/effect since only one ever renders at a time.
  const { error, setError, errorRef } = useErrorAlertFocus();
  const [message, setMessage] = useState('');

  useEffect(() => {
    if (open) {
      setStep('request');
      setEmail('');
      setToken('');
      setNewPassword('');
      setConfirmPassword('');
      setLoading(false);
      setError('');
      setMessage('');
    }
  }, [open, setError]);

  const handleBackToRequest = () => {
    setStep('request');
    setToken('');
    setNewPassword('');
    setConfirmPassword('');
    setError('');
    setMessage('');
  };

  const handleRequest = async (event: FormEvent) => {
    event.preventDefault();
    if (!email) {
      setError(t('passwordReset.validation.emailRequired'));
      return;
    }

    setLoading(true);
    setError('');

    try {
      const responseMessage = await requestPasswordReset(email);
      setMessage(responseMessage);
      setStep('confirm');
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : t('passwordReset.errors.requestFailed');
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleConfirm = async (event: FormEvent) => {
    event.preventDefault();
    if (!token) {
      setError(t('passwordReset.validation.tokenRequired'));
      return;
    }

    if (!newPassword) {
      setError(t('passwordReset.validation.passwordRequired'));
      return;
    }

    if (newPassword !== confirmPassword) {
      setError(t('passwordReset.validation.passwordMismatch'));
      return;
    }

    setLoading(true);
    setError('');

    try {
      const responseMessage = await confirmPasswordReset(token, newPassword);
      setMessage(responseMessage);
      setStep('done');
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : t('passwordReset.errors.confirmFailed');
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const renderContent = () => {
    if (step === 'done') {
      return (
        <Stack spacing={2}>
          <Alert severity="success">{message || t('passwordReset.success')}</Alert>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('passwordReset.doneDescription')}
          </Typography>
        </Stack>
      );
    }

    if (step === 'confirm') {
      return (
        <form onSubmit={handleConfirm}>
          <Stack spacing={2}>
            <Typography
              variant="body2"
              sx={{
                color: 'text.secondary',
              }}
            >
              {t('passwordReset.checkEmail', { email })}
            </Typography>
            {message && <Alert severity="info">{message}</Alert>}
            {error && (
              <Alert severity="error" id="forgot-password-error" ref={errorRef} tabIndex={-1}>
                {error}
              </Alert>
            )}
            <TextField
              label={t('passwordReset.token')}
              value={token}
              onChange={(event) => setToken(event.target.value)}
              fullWidth
              required
              error={Boolean(error)}
              slotProps={{
                htmlInput: { 'aria-describedby': error ? 'forgot-password-error' : undefined },
              }}
            />
            <TextField
              label={t('passwordReset.newPassword')}
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              fullWidth
              required
              error={Boolean(error)}
              slotProps={{
                htmlInput: { 'aria-describedby': error ? 'forgot-password-error' : undefined },
              }}
            />
            <TextField
              label={t('passwordReset.confirmPassword')}
              type="password"
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
              fullWidth
              required
              error={Boolean(error)}
              slotProps={{
                htmlInput: { 'aria-describedby': error ? 'forgot-password-error' : undefined },
              }}
            />
            <Button type="submit" variant="contained" disabled={loading}>
              {loading ? t('passwordReset.confirming') : t('passwordReset.confirmButton')}
            </Button>
          </Stack>
        </form>
      );
    }

    return (
      <form onSubmit={handleRequest}>
        <Stack spacing={2}>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('passwordReset.description')}
          </Typography>
          {error && (
            <Alert severity="error" id="forgot-password-error" ref={errorRef} tabIndex={-1}>
              {error}
            </Alert>
          )}
          <TextField
            label={t('passwordReset.email')}
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            fullWidth
            required
            error={Boolean(error)}
            slotProps={{
              htmlInput: { 'aria-describedby': error ? 'forgot-password-error' : undefined },
            }}
          />
          <Button type="submit" variant="contained" disabled={loading}>
            {loading ? t('passwordReset.requesting') : t('passwordReset.requestButton')}
          </Button>
        </Stack>
      </form>
    );
  };

  const renderActions = () => {
    if (step === 'done') {
      return (
        <DialogActions>
          <Button onClick={onClose} variant="contained">
            {t('passwordReset.closeButton')}
          </Button>
        </DialogActions>
      );
    }

    if (step === 'confirm') {
      return (
        <DialogActions>
          <Button onClick={handleBackToRequest} disabled={loading}>
            {t('passwordReset.backButton')}
          </Button>
          <Button onClick={onClose} disabled={loading}>
            {t('passwordReset.cancelButton')}
          </Button>
        </DialogActions>
      );
    }

    return (
      <DialogActions>
        <Button onClick={onClose} disabled={loading}>
          {t('passwordReset.cancelButton')}
        </Button>
      </DialogActions>
    );
  };

  return (
    <AppDialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{t('passwordReset.title')}</DialogTitle>
      <DialogContent>{renderContent()}</DialogContent>
      {renderActions()}
    </AppDialog>
  );
}
