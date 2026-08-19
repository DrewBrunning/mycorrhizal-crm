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
} from '@mui/material';
import { mdiFileDocumentOutline } from '@mdi/js';
import { SvgIcon } from '@mui/material';
import { usePaperless } from '../hooks/usePaperless';
import { useSnackbar } from '../context/SnackbarContext';
import { isHttpUrlString } from '../utils/linkResolution';

// PaperlessSettings is the settings-page card for the Paperless-ngx connection
// (P2a). The base URL + API token are per-user-global; the token is stored
// encrypted server-side and never shown again after save.
export default function PaperlessSettings() {
  const { t } = useTranslation();
  const { showSuccess, showError } = useSnackbar();
  const paperless = usePaperless({ showError });

  const [baseUrl, setBaseUrl] = useState('');
  const [apiToken, setApiToken] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    paperless.refreshConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (paperless.config) {
      setBaseUrl(paperless.config.base_url || '');
      // The API token is never returned by the backend — leave the field empty
      // so a save without typing one keeps the stored token.
      setApiToken('');
    }
  }, [paperless.config]);

  const handleSave = async () => {
    const trimmed = baseUrl.trim();
    if (!trimmed) {
      setSaveError(t('paperless.settings.baseUrlRequired'));
      return;
    }
    // Mirror the backend's `httpurl` validator (T41) client-side so a
    // non-http(s) base URL is a readable message here rather than a 400.
    if (!isHttpUrlString(trimmed)) {
      setSaveError(t('paperless.settings.invalidBaseUrl'));
      return;
    }
    setSaving(true);
    setSaveError('');
    try {
      const saved = await paperless.saveConfig({
        base_url: trimmed,
        api_token: apiToken.trim() || undefined,
      });
      setApiToken('');
      showSuccess(t('paperless.settings.saved'));
      if (saved) {
        setBaseUrl(saved.base_url);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('paperless.settings.saveFailed');
      setSaveError(msg);
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async () => {
    if (!window.confirm(t('paperless.settings.removeConfirm'))) return;
    try {
      await paperless.removeConfig();
      setBaseUrl('');
      setApiToken('');
      showSuccess(t('paperless.settings.removed'));
    } catch (err) {
      showError(err instanceof Error ? err.message : t('paperless.settings.saveFailed'));
    }
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <SvgIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }}>
            <path d={mdiFileDocumentOutline} />
          </SvgIcon>
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('paperless.settings.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        {paperless.configLoading ? (
          <CircularProgress size={24} />
        ) : (
          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              {t('paperless.settings.description')}
            </Typography>

            <TextField
              label={t('paperless.settings.baseUrl')}
              value={baseUrl}
              onChange={(e) => {
                setBaseUrl(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              placeholder="http://paperless:8000"
            />

            <TextField
              label={t('paperless.settings.apiToken')}
              type="password"
              value={apiToken}
              onChange={(e) => {
                setApiToken(e.target.value);
                setSaveError('');
              }}
              fullWidth
              size="small"
              helperText={
                paperless.config?.has_api_token
                  ? t('paperless.settings.apiTokenHintExisting')
                  : t('paperless.settings.apiTokenHintNew')
              }
            />

            {paperless.testResult && (
              <Alert severity={paperless.testResult.ok ? 'success' : 'warning'} sx={{ py: 0 }}>
                {paperless.testResult.message}
              </Alert>
            )}

            {paperless.configError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {paperless.configError}
              </Alert>
            )}
            {saveError && (
              <Alert severity="error" sx={{ py: 0 }}>
                {saveError}
              </Alert>
            )}

            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button variant="contained" size="small" onClick={handleSave} disabled={saving}>
                {saving ? t('common.saving') : t('paperless.settings.saveButton')}
              </Button>
              {paperless.config?.has_api_token && (
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => {
                    paperless.testConnection().catch(() => {
                      /* handled via the notifier inside the hook */
                    });
                  }}
                  disabled={paperless.testing}
                >
                  {paperless.testing ? t('paperless.settings.testingConnection') : t('paperless.settings.testConnectionButton')}
                </Button>
              )}
              {paperless.config && (
                <Button variant="outlined" size="small" color="error" onClick={handleRemove}>
                  {t('paperless.settings.removeButton')}
                </Button>
              )}
            </Box>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
