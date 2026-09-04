import {
  Alert,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Snackbar,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { type FormEvent, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { onSessionExpired, type SessionExpiryMode } from '../api/sessionExpiry';
import { API_BASE_URL, login2FA, loginUser, logoutAndRedirect } from '../auth';
import { useErrorAlertFocus } from '../hooks/useErrorAlertFocus';
import { useOIDCConfig } from '../hooks/useOIDCConfig';
import AppDialog from './AppDialog';

type ReauthStep = 'credentials' | 'twoFactor';

// Issue #557: the in-app replacement for the old `window.location.href =
// '/login'` hard redirect on a 401. Mounted once at the very root of the
// tree (src/index.tsx), outside the app's own <ErrorBoundary>, so it keeps
// working even if the routed page underneath has crashed or been navigated
// away from -- the two other cases #557 calls out as losing work the same
// way a hard redirect does.
//
// notifySessionExpired's 'passive' mode (a background GET 401'd) surfaces as
// a dismissible, non-modal banner that never steals focus from whatever the
// user is doing. 'blocking' mode (a mutation 401'd -- the user is actively
// waiting on a Save/Delete/Confirm that did not go through) opens a modal
// re-authentication prompt immediately. Either can be resolved in place by
// signing back in, which only refreshes the httpOnly auth cookie -- nothing
// in the React tree remounts, so whatever the user had open survives.
export default function SessionExpiredGate() {
  const { t } = useTranslation();
  const oidcConfig = useOIDCConfig();
  const [visible, setVisible] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [step, setStep] = useState<ReauthStep>('credentials');
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [loading, setLoading] = useState(false);
  const { error, setError, errorRef } = useErrorAlertFocus();

  useEffect(
    () =>
      onSessionExpired(({ mode }: { mode: SessionExpiryMode }) => {
        setVisible(true);
        if (mode === 'blocking') {
          setDialogOpen(true);
        }
      }),
    [],
  );

  const resetForm = () => {
    setStep('credentials');
    setIdentifier('');
    setPassword('');
    setCode('');
    setError('');
    setLoading(false);
  };

  const handleAuthenticated = () => {
    setDialogOpen(false);
    setVisible(false);
    resetForm();
  };

  // "Not now": close the modal without signing in. The 401 that triggered it
  // already failed and stays failed -- this only stops asking right this
  // second. The banner stays up so the user can come back to it once they've
  // finished what they were doing; if they take another mutating action
  // before then, that 401 reopens the modal on its own.
  const handleNotNow = () => {
    setDialogOpen(false);
    resetForm();
  };

  const handleDismissBanner = () => {
    setVisible(false);
  };

  const handleCredentialsSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setLoading(true);
    setError('');
    try {
      const result = await loginUser(identifier, password);
      if (result.two_factor_required) {
        setStep('twoFactor');
        setCode('');
        return;
      }
      handleAuthenticated();
    } catch (err) {
      setError(err instanceof Error ? err.message : t('login.loginFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleCodeSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setLoading(true);
    setError('');
    try {
      await login2FA(code.trim());
      handleAuthenticated();
    } catch (err) {
      const rawMessage = err instanceof Error ? err.message : '';
      setError(rawMessage === 'Invalid code' ? t('login.invalidCode') : rawMessage);
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <Snackbar
        // No autoHideDuration: this is not a routine toast -- it must stay
        // up until the user acts on it or explicitly dismisses it, the same
        // reasoning as ServiceWorkerUpdatePrompt.
        open={visible && !dialogOpen}
        anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
      >
        <Alert
          severity="warning"
          variant="filled"
          sx={{ width: '100%' }}
          onClose={handleDismissBanner}
          action={
            <Button color="inherit" size="small" onClick={() => setDialogOpen(true)}>
              {t('sessionExpired.signIn')}
            </Button>
          }
        >
          {t('sessionExpired.bannerMessage')}
        </Alert>
      </Snackbar>

      <AppDialog open={dialogOpen} onClose={handleNotNow} maxWidth="xs" fullWidth>
        <DialogTitle>{t('sessionExpired.title')}</DialogTitle>
        <DialogContent>
          <Stack spacing={2}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('sessionExpired.description')}
            </Typography>

            {step === 'twoFactor' ? (
              <form onSubmit={handleCodeSubmit}>
                <Stack spacing={2}>
                  <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                    {t('login.twoFactorDescription')}
                  </Typography>
                  <TextField
                    label={t('login.twoFactorCode')}
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    required
                    fullWidth
                    autoFocus
                    error={Boolean(error)}
                    slotProps={{
                      htmlInput: {
                        'aria-describedby': error ? 'session-expired-error' : undefined,
                      },
                    }}
                  />
                  {error && (
                    <Alert severity="error" id="session-expired-error" ref={errorRef} tabIndex={-1}>
                      {error}
                    </Alert>
                  )}
                  <Button type="submit" variant="contained" disabled={loading}>
                    {loading ? t('login.loggingIn') : t('login.loginButton')}
                  </Button>
                </Stack>
              </form>
            ) : (
              <form onSubmit={handleCredentialsSubmit}>
                <Stack spacing={2}>
                  <TextField
                    label={t('login.identifier')}
                    value={identifier}
                    onChange={(e) => setIdentifier(e.target.value)}
                    required
                    fullWidth
                    autoFocus
                    error={Boolean(error)}
                    slotProps={{
                      htmlInput: {
                        'aria-describedby': error ? 'session-expired-error' : undefined,
                      },
                    }}
                  />
                  <TextField
                    label={t('login.password')}
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                    fullWidth
                    error={Boolean(error)}
                    slotProps={{
                      htmlInput: {
                        'aria-describedby': error ? 'session-expired-error' : undefined,
                      },
                    }}
                  />
                  {error && (
                    <Alert severity="error" id="session-expired-error" ref={errorRef} tabIndex={-1}>
                      {error}
                    </Alert>
                  )}
                  <Button type="submit" variant="contained" disabled={loading}>
                    {loading ? t('login.loggingIn') : t('sessionExpired.signIn')}
                  </Button>
                  {oidcConfig.enabled && (
                    <>
                      <Divider>{t('login.orSeparator')}</Divider>
                      <Button
                        variant="outlined"
                        onClick={() => {
                          window.location.href = `${API_BASE_URL}/auth/oidc/login`;
                        }}
                      >
                        {t('sessionExpired.ssoButton', { provider: oidcConfig.provider_name })}
                      </Button>
                    </>
                  )}
                </Stack>
              </form>
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Box sx={{ flex: 1 }} />
          <Button onClick={handleNotNow} disabled={loading}>
            {t('sessionExpired.notNow')}
          </Button>
          <Button onClick={() => void logoutAndRedirect()} disabled={loading}>
            {t('sessionExpired.logOut')}
          </Button>
        </DialogActions>
      </AppDialog>
    </>
  );
}
