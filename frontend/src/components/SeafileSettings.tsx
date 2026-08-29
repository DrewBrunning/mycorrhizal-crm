import { mdiFolderMultipleOutline } from '@mdi/js';
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
import { useSeafile } from '../hooks/useSeafile';
import { isHttpUrlString } from '../utils/linkResolution';

// SeafileSettings is the settings-page card for the Seafile connection (P2b).
// The server URL + API token are per-user-global; the token is stored
// encrypted server-side and never shown again after save.
export default function SeafileSettings() {
  const { t } = useTranslation();
  const { showSuccess, showError } = useSnackbar();
  const seafile = useSeafile({ showError });

  const [baseUrl, setBaseUrl] = useState('');
  const [apiToken, setApiToken] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    seafile.refreshConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (seafile.config) {
      setBaseUrl(seafile.config.base_url || '');
      // The API token is never returned by the backend — leave the field empty
      // so a save without typing one keeps the stored token.
      setApiToken('');
    }
  }, [seafile.config]);

  const handleSave = async () => {
    const trimmed = baseUrl.trim();
    if (!trimmed) {
      setSaveError(t('seafile.settings.baseUrlRequired'));
      return;
    }
    // Mirror the backend's `httpurl` validator (T41) client-side.
    if (!isHttpUrlString(trimmed)) {
      setSaveError(t('seafile.settings.invalidBaseUrl'));
      return;
    }
    setSaving(true);
    setSaveError('');
    try {
      const saved = await seafile.saveConfig({
        base_url: trimmed,
        api_token: apiToken.trim() || undefined,
      });
      setApiToken('');
      showSuccess(t('seafile.settings.saved'));
      if (saved) {
        setBaseUrl(saved.base_url);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('seafile.settings.saveFailed');
      setSaveError(msg);
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async () => {
    if (!window.confirm(t('seafile.settings.removeConfirm'))) return;
    try {
      await seafile.removeConfig();
      setBaseUrl('');
      setApiToken('');
      showSuccess(t('seafile.settings.removed'));
    } catch (err) {
      showError(err instanceof Error ? err.message : t('seafile.settings.saveFailed'));
    }
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <SvgIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }}>
            <path d={mdiFolderMultipleOutline} />
          </SvgIcon>
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('seafile.settings.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {seafile.configLoading ? (
          <CircularProgress size={24} />
        ) : (
          <Stack spacing={1.5}>
            <Typography
              variant="body2"
              sx={{
                color: 'text.secondary',
              }}
            >
              {t('seafile.settings.description')}
            </Typography>

            <TextField
              label={t('seafile.settings.baseUrl')}
              value={baseUrl}
              onChange={(e) => {
                setBaseUrl(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              placeholder="https://seafile.example.com"
            />

            <TextField
              label={t('seafile.settings.apiToken')}
              type="password"
              value={apiToken}
              onChange={(e) => {
                setApiToken(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              helperText={
                seafile.config?.has_api_token
                  ? t('seafile.settings.apiTokenHintExisting')
                  : t('seafile.settings.apiTokenHintNew')
              }
            />

            {seafile.testResult && (
              <Alert severity={seafile.testResult.ok ? 'success' : 'warning'} sx={{ py: 0 }}>
                {seafile.testResult.message}
              </Alert>
            )}

            {seafile.configError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {seafile.configError}
              </Alert>
            )}
            {saveError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {saveError}
              </Alert>
            )}

            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button variant="contained" size="small" onClick={handleSave} disabled={saving}>
                {saving ? t('common.saving') : t('seafile.settings.saveButton')}
              </Button>
              {seafile.config?.has_api_token && (
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => {
                    seafile.testConnection().catch(() => {
                      /* handled via the notifier inside the hook */
                    });
                  }}
                  disabled={seafile.testing}
                >
                  {seafile.testing
                    ? t('seafile.settings.testingConnection')
                    : t('seafile.settings.testConnectionButton')}
                </Button>
              )}
              {seafile.config && (
                <Button variant="outlined" size="small" color="error" onClick={handleRemove}>
                  {t('seafile.settings.removeButton')}
                </Button>
              )}
            </Box>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
