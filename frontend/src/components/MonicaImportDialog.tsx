import {
  Alert,
  Box,
  Button,
  CircularProgress,
  FormControlLabel,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  connectMonica,
  MONICA_IMPORT_BASE,
  type MonicaConnectResponse,
  startMonicaFetch,
} from '../api/monicaImport';
import { useSourceImportWizard } from '../hooks/useSourceImportWizard';
import { getErrorMessage } from '../utils/errorHandler';
import SourceImportWizard from './sourceImport/SourceImportWizard';

interface Props {
  open: boolean;
  onClose: () => void;
  onImportComplete: () => void;
}

// The Monica import assistant (issue #549). Everything after "connect" is the
// shared SourceImportWizard; this component only owns the Monica-specific
// step 0 — instance URL + API token + fetch options — and hands the wizard a
// session id once connected. Web-only, by design (consistent with the rest of
// import).
export default function MonicaImportDialog({ open, onClose, onImportComplete }: Props) {
  const { t } = useTranslation();

  const [baseUrl, setBaseUrl] = useState('');
  const [apiToken, setApiToken] = useState('');
  const [includeRelationships, setIncludeRelationships] = useState(true);
  const [includeExtras, setIncludeExtras] = useState(true);
  const [connecting, setConnecting] = useState(false);
  const [connectError, setConnectError] = useState<string | null>(null);
  const [connectResp, setConnectResp] = useState<MonicaConnectResponse | null>(null);

  // beginFetch runs after connect, so the wizard's startFetch reads the
  // options through a ref rather than a stale closure.
  const optsRef = useRef({ include_relationships: true, include_extras: true });
  useEffect(() => {
    optsRef.current = {
      include_relationships: includeRelationships,
      include_extras: includeExtras,
    };
  }, [includeRelationships, includeExtras]);

  const startFetch = useCallback(
    (sessionId: string) => startMonicaFetch(sessionId, optsRef.current),
    [],
  );
  const wizard = useSourceImportWizard({ basePath: MONICA_IMPORT_BASE, startFetch });

  const resetLocal = useCallback(() => {
    setBaseUrl('');
    setApiToken('');
    setIncludeRelationships(true);
    setIncludeExtras(true);
    setConnecting(false);
    setConnectError(null);
    setConnectResp(null);
  }, []);

  const handleClose = useCallback(() => {
    resetLocal();
    onClose();
  }, [onClose, resetLocal]);

  const handleComplete = useCallback(() => {
    resetLocal();
    onImportComplete();
  }, [onImportComplete, resetLocal]);

  const handleConnect = async () => {
    setConnecting(true);
    setConnectError(null);
    try {
      const resp = await connectMonica(baseUrl.trim(), apiToken);
      setConnectResp(resp);
      // The token has done its job client-side; the server holds it now.
      setApiToken('');
    } catch (err) {
      setConnectError(getErrorMessage(err));
    } finally {
      setConnecting(false);
    }
  };

  const handleStart = () => {
    if (connectResp) void wizard.beginFetch(connectResp.session_id);
  };

  const connectStep = (
    <Stack spacing={2}>
      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
        {t('settings.monicaImport.connect.description')}
      </Typography>
      {connectError && <Alert severity="error">{connectError}</Alert>}

      <TextField
        label={t('settings.monicaImport.connect.urlLabel')}
        placeholder="https://app.monicahq.com"
        value={baseUrl}
        onChange={(e) => setBaseUrl(e.target.value)}
        fullWidth
        disabled={!!connectResp}
        helperText={t('settings.monicaImport.connect.urlHelp')}
      />
      <TextField
        label={t('settings.monicaImport.connect.tokenLabel')}
        type="password"
        value={apiToken}
        onChange={(e) => setApiToken(e.target.value)}
        fullWidth
        disabled={!!connectResp}
        autoComplete="off"
        helperText={t('settings.monicaImport.connect.tokenHelp')}
      />

      <Box>
        <FormControlLabel
          control={
            <Switch
              checked={includeRelationships}
              onChange={(e) => setIncludeRelationships(e.target.checked)}
              disabled={!!connectResp}
            />
          }
          label={t('settings.monicaImport.connect.includeRelationships')}
        />
        <FormControlLabel
          control={
            <Switch
              checked={includeExtras}
              onChange={(e) => setIncludeExtras(e.target.checked)}
              disabled={!!connectResp}
            />
          }
          label={t('settings.monicaImport.connect.includeExtras')}
        />
      </Box>

      {!connectResp ? (
        <Box>
          <Button
            variant="contained"
            onClick={handleConnect}
            disabled={connecting || !baseUrl.trim() || !apiToken}
            startIcon={connecting ? <CircularProgress size={16} color="inherit" /> : undefined}
          >
            {connecting
              ? t('settings.monicaImport.connect.connecting')
              : t('settings.monicaImport.connect.connect')}
          </Button>
        </Box>
      ) : (
        <Alert severity="info" sx={{ '& .MuiAlert-message': { width: '100%' } }}>
          <Typography variant="body2" gutterBottom>
            {t('settings.monicaImport.connect.found', {
              contacts: connectResp.totals.contacts,
              activities: connectResp.totals.activities,
              notes: connectResp.totals.notes,
            })}
          </Typography>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {t('settings.monicaImport.connect.estimate', {
              minutes: Math.max(1, Math.round(connectResp.estimated_fetch_seconds / 60)),
            })}
          </Typography>
          <Box sx={{ mt: 1 }}>
            <Button variant="contained" onClick={handleStart} disabled={wizard.busy}>
              {t('settings.monicaImport.connect.start')}
            </Button>
          </Box>
        </Alert>
      )}
    </Stack>
  );

  return (
    <SourceImportWizard
      open={open}
      onClose={handleClose}
      titleKey="settings.monicaImport.title"
      sourceLabel="Monica"
      wizard={wizard}
      connectStep={connectStep}
      onComplete={handleComplete}
    />
  );
}
