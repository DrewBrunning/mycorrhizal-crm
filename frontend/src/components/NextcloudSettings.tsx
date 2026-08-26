import { mdiCloudOutline } from '@mdi/js';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  Stack,
  SvgIcon,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSnackbar } from '../context/SnackbarContext';
import { useNextcloud } from '../hooks/useNextcloud';
import { isHttpUrlString } from '../utils/linkResolution';

// NextcloudSettings is the settings-page card for the Nextcloud / ownCloud
// (WebDAV) connection (P2c). The base URL + username + app password are
// per-user-global; the app password is stored encrypted server-side and never
// shown again after save. Only an app password is accepted — never the
// account password.
export default function NextcloudSettings() {
  const { t } = useTranslation();
  const { showSuccess, showError } = useSnackbar();
  const nextcloud = useNextcloud({ showError });

  const [baseUrl, setBaseUrl] = useState('');
  const [username, setUsername] = useState('');
  const [appPassword, setAppPassword] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    nextcloud.refreshConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (nextcloud.config) {
      setBaseUrl(nextcloud.config.base_url || '');
      setUsername(nextcloud.config.username || '');
      // The app password is never returned by the backend — leave the field
      // empty so a save without typing one keeps the stored password.
      setAppPassword('');
    }
  }, [nextcloud.config]);

  const handleSave = async () => {
    const trimmed = baseUrl.trim();
    if (!trimmed) {
      setSaveError(t('nextcloud.settings.baseUrlRequired'));
      return;
    }
    if (!username.trim()) {
      setSaveError(t('nextcloud.settings.usernameRequired'));
      return;
    }
    // Mirror the backend's `httpurl` validator (T41) client-side.
    if (!isHttpUrlString(trimmed)) {
      setSaveError(t('nextcloud.settings.invalidBaseUrl'));
      return;
    }
    setSaving(true);
    setSaveError('');
    try {
      const saved = await nextcloud.saveConfig({
        base_url: trimmed,
        username: username.trim(),
        app_password: appPassword.trim() || undefined,
      });
      setAppPassword('');
      showSuccess(t('nextcloud.settings.saved'));
      if (saved) {
        setBaseUrl(saved.base_url);
        setUsername(saved.username);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('nextcloud.settings.saveFailed');
      setSaveError(msg);
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async () => {
    if (!window.confirm(t('nextcloud.settings.removeConfirm'))) return;
    try {
      await nextcloud.removeConfig();
      setBaseUrl('');
      setUsername('');
      setAppPassword('');
      showSuccess(t('nextcloud.settings.removed'));
    } catch (err) {
      showError(err instanceof Error ? err.message : t('nextcloud.settings.saveFailed'));
    }
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <SvgIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }}>
            <path d={mdiCloudOutline} />
          </SvgIcon>
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('nextcloud.settings.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {nextcloud.configLoading ? (
          <CircularProgress size={24} />
        ) : (
          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              {t('nextcloud.settings.description')}
            </Typography>

            <TextField
              label={t('nextcloud.settings.baseUrl')}
              value={baseUrl}
              onChange={(e) => {
                setBaseUrl(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              placeholder="https://nextcloud.example.com"
            />

            <TextField
              label={t('nextcloud.settings.username')}
              value={username}
              onChange={(e) => {
                setUsername(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
            />

            <TextField
              label={t('nextcloud.settings.appPassword')}
              type="password"
              value={appPassword}
              onChange={(e) => {
                setAppPassword(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              helperText={
                nextcloud.config?.has_app_password
                  ? t('nextcloud.settings.appPasswordHintExisting')
                  : t('nextcloud.settings.appPasswordHintNew')
              }
            />

            {nextcloud.testResult && (
              <Alert severity={nextcloud.testResult.ok ? 'success' : 'warning'} sx={{ py: 0 }}>
                {nextcloud.testResult.message}
              </Alert>
            )}

            {nextcloud.configError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {nextcloud.configError}
              </Alert>
            )}
            {saveError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {saveError}
              </Alert>
            )}

            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button variant="contained" size="small" onClick={handleSave} disabled={saving}>
                {saving ? t('common.saving') : t('nextcloud.settings.saveButton')}
              </Button>
              {nextcloud.config?.has_app_password && (
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => {
                    nextcloud.testConnection().catch(() => {
                      /* handled via the notifier inside the hook */
                    });
                  }}
                  disabled={nextcloud.testing}
                >
                  {nextcloud.testing
                    ? t('nextcloud.settings.testingConnection')
                    : t('nextcloud.settings.testConnectionButton')}
                </Button>
              )}
              {nextcloud.config && (
                <Button variant="outlined" size="small" color="error" onClick={handleRemove}>
                  {t('nextcloud.settings.removeButton')}
                </Button>
              )}
            </Box>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
