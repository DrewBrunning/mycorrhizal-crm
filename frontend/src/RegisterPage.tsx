import { Alert, Box, Button, Paper, Stack, TextField, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router';
import { API_BASE_URL } from './auth';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useErrorAlertFocus } from './hooks/useErrorAlertFocus';
import { useOIDCConfig } from './hooks/useOIDCConfig';

export default function RegisterPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('register.title'));
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [username, setUsername] = useState('');
  // #192: same fix as LoginPage.tsx -- move focus to the error and
  // associate it with the fields instead of dropping focus to <body>.
  const { error, setError, errorRef } = useErrorAlertFocus();
  const [success, setSuccess] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const oidcConfig = useOIDCConfig();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    setSuccess('');
    try {
      const response = await fetch(`${API_BASE_URL}/register`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ email, password, username }),
      });
      if (!response.ok) {
        const data = await response.json();
        const apiError = data.error;
        if (apiError) {
          const baseMessage = apiError.message || '';
          let detailMessages = '';

          if (apiError.details && Object.keys(apiError.details).length > 0) {
            // Flatten detail values into a human-readable string.
            detailMessages = Object.values(apiError.details)
              .flatMap((value) => (Array.isArray(value) ? value : [value]))
              .filter(Boolean)
              .join('. ');
          }

          const combinedMessage = [baseMessage, detailMessages].filter(Boolean).join(': ');

          throw new Error(combinedMessage || t('register.registrationFailed'));
        }

        throw new Error(t('register.registrationFailed'));
      }
      setSuccess(t('register.registrationSuccess'));
      setTimeout(() => navigate('/login'), 1500);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : t('register.registrationFailed');
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  // Registration is disabled server-side (DISABLE_REGISTRATION): show that
  // plainly instead of a form that would always 403 on submit — someone can
  // still land here directly via a bookmarked/typed /register URL even
  // though LoginPage now hides the link to it.
  if (oidcConfig.registration_disabled) {
    return (
      <Box sx={{ maxWidth: 400, mx: 'auto', mt: 8 }}>
        <Paper sx={{ p: 4 }}>
          <Typography variant="h5" component="h1" mb={2}>
            {t('register.title')}
          </Typography>
          <Alert severity="info">{t('register.registrationDisabled')}</Alert>
          <Button component={Link} to="/login" color="primary" variant="text" sx={{ mt: 2 }}>
            {t('register.backToLogin')}
          </Button>
        </Paper>
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 400, mx: 'auto', mt: 8 }}>
      <Paper sx={{ p: 4 }}>
        <Typography variant="h5" component="h1" mb={2}>
          {t('register.title')}
        </Typography>
        <form onSubmit={handleSubmit}>
          <Stack spacing={2}>
            <TextField
              label={t('register.username')}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              fullWidth
              error={Boolean(error)}
              inputProps={{ 'aria-describedby': error ? 'register-error' : undefined }}
            />
            <TextField
              label={t('register.email')}
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              fullWidth
              error={Boolean(error)}
              inputProps={{ 'aria-describedby': error ? 'register-error' : undefined }}
            />
            <TextField
              label={t('register.password')}
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              fullWidth
              error={Boolean(error)}
              inputProps={{ 'aria-describedby': error ? 'register-error' : undefined }}
            />
            {error && (
              <Alert severity="error" id="register-error" ref={errorRef} tabIndex={-1}>
                {error}
              </Alert>
            )}
            {success && <Alert severity="success">{success}</Alert>}
            <Button type="submit" variant="contained" color="primary" disabled={loading}>
              {loading ? t('register.registering') : t('register.registerButton')}
            </Button>
          </Stack>
        </form>
      </Paper>
    </Box>
  );
}
