import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Divider,
  TextField,
  Button,
  Stack,
  Alert,
  CircularProgress,
  FormControlLabel,
  Switch,
} from '@mui/material';
import { mdiImageMultipleOutline } from '@mdi/js';
import { SvgIcon } from '@mui/material';
import { useImmich } from '../hooks/useImmich';
import { useSnackbar } from '../context/SnackbarContext';
import { isHttpUrlString } from '../utils/linkResolution';

// ImmichSettings is the settings-page card for the Immich connection
// (T15/T16). The base URL + API key are per-user-global; the key is stored
// encrypted server-side and never shown again after save.
export default function ImmichSettings() {
  const { t } = useTranslation();
  const { showSuccess, showError } = useSnackbar();
  const immich = useImmich({ showError });

  const [baseUrl, setBaseUrl] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [syncEnabled, setSyncEnabled] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    immich.refreshConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (immich.config) {
      setBaseUrl(immich.config.base_url || '');
      setSyncEnabled(immich.config.sync_enabled);
      // The API key is never returned by the backend — leave the field empty
      // so a save without typing one keeps the stored key.
      setApiKey('');
    }
  }, [immich.config]);

  const handleSave = async () => {
    const trimmed = baseUrl.trim();
    if (!trimmed) {
      setSaveError(t('immich.settings.baseUrlRequired'));
      return;
    }
    // Mirror the backend's `httpurl` validator (T41) client-side so a
    // non-http(s) base URL is a readable message here rather than a 400.
    // Deliberately no scheme-less→https default like GiftDialog: the backend
    // service rejects a missing scheme (there is no way to guess http vs
    // https), so the client must too.
    if (!isHttpUrlString(trimmed)) {
      setSaveError(t('immich.settings.invalidBaseUrl'));
      return;
    }
    setSaving(true);
    setSaveError('');
    try {
      const saved = await immich.saveConfig({
        base_url: trimmed,
        api_key: apiKey.trim() || undefined,
        sync_enabled: syncEnabled,
      });
      setApiKey('');
      showSuccess(t('immich.settings.saved'));
      if (saved) {
        setBaseUrl(saved.base_url);
        setSyncEnabled(saved.sync_enabled);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('immich.settings.saveFailed');
      setSaveError(msg);
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async () => {
    if (!window.confirm(t('immich.settings.removeConfirm'))) return;
    try {
      await immich.removeConfig();
      setBaseUrl('');
      setApiKey('');
      showSuccess(t('immich.settings.removed'));
    } catch (err) {
      showError(err instanceof Error ? err.message : t('immich.settings.saveFailed'));
    }
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <SvgIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }}>
            <path d={mdiImageMultipleOutline} />
          </SvgIcon>
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('immich.settings.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {immich.configLoading ? (
          <CircularProgress size={24} />
        ) : (
          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              {t('immich.settings.description')}
            </Typography>

            <TextField
              label={t('immich.settings.baseUrl')}
              value={baseUrl}
              onChange={(e) => {
                setBaseUrl(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              placeholder="http://immich:2283"
            />

            <TextField
              label={t('immich.settings.apiKey')}
              type="password"
              value={apiKey}
              onChange={(e) => {
                setApiKey(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              helperText={
                immich.config?.has_api_key
                  ? t('immich.settings.apiKeyHintExisting')
                  : t('immich.settings.apiKeyHintNew')
              }
            />

            {immich.config?.has_api_key && (
              <FormControlLabel
                control={
                  <Switch
                    checked={syncEnabled}
                    onChange={(e) => setSyncEnabled(e.target.checked)}
                  />
                }
                label={t('immich.settings.syncEnabled')}
              />
            )}

            {immich.config?.last_sync_status === 'error' && (
              <Alert severity="warning" sx={{ py: 0 }}>
                {t('immich.settings.lastSyncError')}
                {immich.config.last_sync_error ? ` — ${immich.config.last_sync_error}` : ''}
              </Alert>
            )}

            {immich.testResult && (
              <Alert severity={immich.testResult.ok ? 'success' : 'warning'} sx={{ py: 0 }}>
                {immich.testResult.message}
              </Alert>
            )}

            {immich.configError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {immich.configError}
              </Alert>
            )}
            {saveError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {saveError}
              </Alert>
            )}

            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button variant="contained" size="small" onClick={handleSave} disabled={saving}>
                {saving ? t('common.saving') : t('immich.settings.saveButton')}
              </Button>
              {immich.config?.has_api_key && (
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => {
                    immich.testConnection().catch(() => {
                      /* handled via the notifier inside the hook */
                    });
                  }}
                  disabled={immich.testing}
                >
                  {immich.testing ? t('immich.settings.testingConnection') : t('immich.settings.testConnectionButton')}
                </Button>
              )}
              {immich.config && (
                <Button variant="outlined" size="small" color="error" onClick={handleRemove}>
                  {t('immich.settings.removeButton')}
                </Button>
              )}
            </Box>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
