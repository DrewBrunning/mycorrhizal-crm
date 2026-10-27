import {
  Alert,
  Box,
  Button,
  Checkbox,
  CircularProgress,
  FormControlLabel,
  Link,
  Radio,
  RadioGroup,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { connectMeerkat, type MeerkatConnectResponse } from '../api/meerkatImport';
import { getErrorMessage } from '../utils/errorHandler';

interface Props {
  connected: MeerkatConnectResponse | null;
  onConnected: (resp: MeerkatConnectResponse) => void;
  acceptUntested: boolean;
  onAcceptUntestedChange: (accept: boolean) => void;
  onStart: () => void;
  starting: boolean;
  onUseFile: () => void;
}

type AuthMode = 'token' | 'password';

// Step 0 of the live Meerkat import (issue #2013): address + credentials, then
// the detected version and counts. The credential is sent once and wiped from
// the form as soon as the server holds it — it is never kept client-side.
export default function MeerkatLiveConnect({
  connected,
  onConnected,
  acceptUntested,
  onAcceptUntestedChange,
  onStart,
  starting,
  onUseFile,
}: Props) {
  const { t } = useTranslation();
  const [baseUrl, setBaseUrl] = useState('');
  const [authMode, setAuthMode] = useState<AuthMode>('token');
  const [apiToken, setApiToken] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const hasCredential = authMode === 'token' ? !!apiToken : !!username.trim() && !!password;

  const handleConnect = async () => {
    setConnecting(true);
    setError(null);
    try {
      const base_url = baseUrl.trim();
      const resp = await connectMeerkat(
        authMode === 'token'
          ? { base_url, api_token: apiToken }
          : { base_url, username: username.trim(), password },
      );
      setApiToken('');
      setPassword('');
      onConnected(resp);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setConnecting(false);
    }
  };

  const locked = !!connected;

  return (
    <Stack spacing={2}>
      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
        {t('settings.meerkatImport.live.description')}
      </Typography>
      {error && (
        <Alert severity="error">
          {error}{' '}
          <Link component="button" type="button" onClick={onUseFile}>
            {t('settings.meerkatImport.live.useFile')}
          </Link>
        </Alert>
      )}

      <TextField
        label={t('settings.meerkatImport.live.urlLabel')}
        placeholder="https://meerkat.example.com"
        value={baseUrl}
        onChange={(e) => setBaseUrl(e.target.value)}
        fullWidth
        disabled={locked}
        helperText={t('settings.meerkatImport.live.urlHelp')}
      />

      <RadioGroup
        row
        value={authMode}
        onChange={(e) => setAuthMode(e.target.value as AuthMode)}
        aria-label={t('settings.meerkatImport.live.authMode')}
      >
        <FormControlLabel
          value="token"
          control={<Radio size="small" disabled={locked} />}
          label={t('settings.meerkatImport.live.modeToken')}
        />
        <FormControlLabel
          value="password"
          control={<Radio size="small" disabled={locked} />}
          label={t('settings.meerkatImport.live.modePassword')}
        />
      </RadioGroup>

      {authMode === 'token' ? (
        <TextField
          label={t('settings.meerkatImport.live.tokenLabel')}
          type="password"
          value={apiToken}
          onChange={(e) => setApiToken(e.target.value)}
          fullWidth
          disabled={locked}
          autoComplete="off"
          helperText={t('settings.meerkatImport.live.tokenHelp')}
        />
      ) : (
        <>
          <TextField
            label={t('settings.meerkatImport.live.usernameLabel')}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            fullWidth
            disabled={locked}
            autoComplete="off"
          />
          <TextField
            label={t('settings.meerkatImport.live.passwordLabel')}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            fullWidth
            disabled={locked}
            autoComplete="off"
            helperText={t('settings.meerkatImport.live.passwordHelp')}
          />
        </>
      )}

      {!connected ? (
        <Box>
          <Button
            variant="contained"
            onClick={() => void handleConnect()}
            disabled={connecting || !baseUrl.trim() || !hasCredential}
            startIcon={connecting ? <CircularProgress size={16} color="inherit" /> : undefined}
          >
            {connecting
              ? t('settings.meerkatImport.live.connecting')
              : t('settings.meerkatImport.live.connect')}
          </Button>
        </Box>
      ) : (
        <Stack spacing={1}>
          {connected.insecure_transport && (
            <Alert severity="warning">{t('settings.meerkatImport.live.insecure')}</Alert>
          )}
          {!connected.version.supported && (
            <Alert severity="warning">
              <Typography variant="body2">
                {t('settings.meerkatImport.live.untested', {
                  reason: connected.version.reason,
                })}
              </Typography>
              <FormControlLabel
                control={
                  <Checkbox
                    size="small"
                    checked={acceptUntested}
                    onChange={(e) => onAcceptUntestedChange(e.target.checked)}
                  />
                }
                label={t('settings.meerkatImport.live.tryAnyway')}
              />
            </Alert>
          )}
          <Alert severity="info" sx={{ '& .MuiAlert-message': { width: '100%' } }}>
            {/* Connect only learns the contact total; relationships and notes
                are counted once the fetch has read them. */}
            <Typography variant="body2" gutterBottom>
              {t('settings.meerkatImport.live.found', { count: connected.totals.contacts ?? 0 })}
            </Typography>
            {connected.version.supported && (
              <Typography variant="caption" sx={{ display: 'block', color: 'text.secondary' }}>
                {t('settings.meerkatImport.live.version', {
                  generation: connected.version.generation,
                })}
              </Typography>
            )}
            <Typography variant="caption" sx={{ display: 'block', color: 'text.secondary' }}>
              {t('settings.meerkatImport.live.estimate', {
                minutes: Math.max(1, Math.round(connected.estimated_fetch_seconds / 60)),
              })}
            </Typography>
            <Box sx={{ mt: 1 }}>
              <Button
                variant="contained"
                onClick={onStart}
                disabled={starting || (!connected.version.supported && !acceptUntested)}
              >
                {t('settings.meerkatImport.connect.start')}
              </Button>
            </Box>
          </Alert>
        </Stack>
      )}
    </Stack>
  );
}
