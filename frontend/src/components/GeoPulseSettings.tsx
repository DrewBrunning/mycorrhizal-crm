import { mdiMapMarkerOutline } from '@mdi/js';
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
import { useGeoPulse } from '../hooks/useGeoPulse';
import { isHttpUrlString } from '../utils/linkResolution';
import { secretRequiredForOriginChange } from '../utils/urlOrigin';

// GeoPulseSettings is the settings-page card for the GeoPulse connection (issue
// #160, ADR 0033). The base URL + API token are per-user-global; the token is
// stored encrypted server-side and never shown again after save. GeoPulse's own
// user id is not asked for — the server discovers it.

export default function GeoPulseSettings() {
  const { t } = useTranslation();
  const { showSuccess, showError } = useSnackbar();
  const geopulse = useGeoPulse({ showError });

  const [baseUrl, setBaseUrl] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  // Mirrors the backend rule: moving a stored connection to a different origin
  // requires re-entering the token (it would otherwise be sent to the new host).
  const tokenRequired = secretRequiredForOriginChange(
    geopulse.config?.has_api_key,
    geopulse.config?.base_url,
    baseUrl,
  );

  useEffect(() => {
    void geopulse.refreshConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (geopulse.config) {
      setBaseUrl(geopulse.config.base_url || '');
      // The API token is never returned by the backend — leave the field empty
      // so a save without typing one keeps the stored token.
      setApiKey('');
    }
  }, [geopulse.config]);

  const handleSave = async () => {
    const trimmed = baseUrl.trim();
    if (!trimmed) {
      setSaveError(t('geopulse.settings.baseUrlRequired'));
      return;
    }
    // Mirror the backend's `httpurl` validator client-side so a non-http(s)
    // base URL is a readable message here rather than a 400.
    if (!isHttpUrlString(trimmed)) {
      setSaveError(t('geopulse.settings.invalidBaseUrl'));
      return;
    }
    if (tokenRequired && !apiKey.trim()) {
      setSaveError(t('geopulse.settings.apiKeyRequiredOriginChange'));
      return;
    }
    setSaving(true);
    setSaveError('');
    try {
      const saved = await geopulse.saveConfig({
        base_url: trimmed,
        api_key: apiKey.trim() || undefined,
      });
      setApiKey('');
      showSuccess(t('geopulse.settings.saved'));
      if (saved) {
        setBaseUrl(saved.base_url);
      }
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : t('geopulse.settings.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async () => {
    if (!window.confirm(t('geopulse.settings.removeConfirm'))) return;
    try {
      await geopulse.removeConfig();
      setBaseUrl('');
      setApiKey('');
      showSuccess(t('geopulse.settings.removed'));
    } catch (err) {
      showError(err instanceof Error ? err.message : t('geopulse.settings.saveFailed'));
    }
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <SvgIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }}>
            <path d={mdiMapMarkerOutline} />
          </SvgIcon>
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('geopulse.settings.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {geopulse.configLoading ? (
          <CircularProgress size={24} />
        ) : (
          <Stack spacing={1.5}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('geopulse.settings.description')}
            </Typography>

            <TextField
              label={t('geopulse.settings.baseUrl')}
              value={baseUrl}
              onChange={(e) => {
                setBaseUrl(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              placeholder="http://geopulse:8080"
            />

            <TextField
              label={t('geopulse.settings.apiKey')}
              type="password"
              value={apiKey}
              onChange={(e) => {
                setApiKey(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              required={tokenRequired}
              helperText={
                tokenRequired
                  ? t('geopulse.settings.apiKeyRequiredOriginChange')
                  : geopulse.config?.has_api_key
                    ? t('geopulse.settings.apiKeyHintExisting')
                    : t('geopulse.settings.apiKeyHintNew')
              }
            />

            {geopulse.testResult && (
              <Alert severity={geopulse.testResult.ok ? 'success' : 'warning'} sx={{ py: 0 }}>
                {geopulse.testResult.message}
              </Alert>
            )}

            {geopulse.configError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {geopulse.configError}
              </Alert>
            )}
            {saveError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {saveError}
              </Alert>
            )}

            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button
                variant="contained"
                size="small"
                onClick={() => void handleSave()}
                disabled={saving}
              >
                {saving ? t('common.saving') : t('geopulse.settings.saveButton')}
              </Button>
              {geopulse.config?.has_api_key && (
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => {
                    geopulse.testConnection().catch(() => {
                      /* handled via the notifier inside the hook */
                    });
                  }}
                  disabled={geopulse.testing}
                >
                  {geopulse.testing
                    ? t('geopulse.settings.testingConnection')
                    : t('geopulse.settings.testConnectionButton')}
                </Button>
              )}
              {geopulse.config?.has_api_key && (
                <Button
                  variant="outlined"
                  size="small"
                  color="error"
                  onClick={() => void handleRemove()}
                >
                  {t('geopulse.settings.removeButton')}
                </Button>
              )}
            </Box>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
