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
import type { TFunction } from 'i18next';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api/client';
import {
  cancelMonicaImport,
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

// A failed Connect, ready to show. `field: 'url'` attaches it to the address
// field (#2680); otherwise it is a banner, with the request ref on its own
// secondary line when the server sent one.
interface ConnectError {
  message: string;
  field?: 'url';
  ref?: string;
}

// The reason codes that are about the address the user typed.
const URL_REASONS = new Set(['monica_url_invalid', 'monica_url_private', 'monica_not_monica_api']);

function hostOf(url: string): string {
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

function describeConnectError(err: unknown, baseUrl: string, t: TFunction): ConnectError {
  if (err instanceof ApiError) {
    if (err.reasonCode === 'monica_unreachable') {
      // The registry text is the raw "monica service error: ..." sentence;
      // say which address could not be reached instead (#2680).
      return {
        message: t('settings.monicaImport.connect.unreachable', { host: hostOf(baseUrl) }),
        ref: err.requestId ? t('errors.requestRef', { id: err.requestId }) : undefined,
      };
    }
    if (err.reasonCode && URL_REASONS.has(err.reasonCode)) {
      return { message: getErrorMessage(err), field: 'url' };
    }
  }
  return { message: getErrorMessage(err) };
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
  const [connectError, setConnectError] = useState<ConnectError | null>(null);
  const [connectResp, setConnectResp] = useState<MonicaConnectResponse | null>(null);
  // Bumped on close. Cancel stays enabled during Connect, so a connect that
  // resolves after the dialog closed belongs to a dead attempt: its session is
  // released and its result dropped (issue #2916).
  const attempt = useRef(0);

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
    attempt.current += 1;
    // A session the wizard never received (Start not pressed) is invisible to
    // wizard.cancel(), which would leave it open on the server until it
    // expires (issue #2916, mirroring MeerkatImportDialog).
    if (connectResp?.session_id && connectResp.session_id !== wizard.sessionId) {
      void cancelMonicaImport(connectResp.session_id).catch(() => undefined);
    }
    resetLocal();
    onClose();
  }, [connectResp, wizard.sessionId, onClose, resetLocal]);

  const handleComplete = useCallback(() => {
    resetLocal();
    onImportComplete();
  }, [onImportComplete, resetLocal]);

  const handleConnect = async () => {
    const mine = attempt.current;
    setConnecting(true);
    setConnectError(null);
    try {
      const resp = await connectMonica(baseUrl.trim(), apiToken);
      if (attempt.current !== mine) {
        void cancelMonicaImport(resp.session_id).catch(() => undefined);
        return;
      }
      setConnectResp(resp);
      // The token has done its job client-side; the server holds it now.
      setApiToken('');
    } catch (err) {
      if (attempt.current !== mine) return;
      setConnectError(describeConnectError(err, baseUrl.trim(), t));
    } finally {
      if (attempt.current === mine) setConnecting(false);
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
      {connectError?.field === undefined && connectError && (
        <Alert severity="error">
          {connectError.message}
          {connectError.ref && (
            <Typography variant="caption" sx={{ display: 'block' }}>
              {connectError.ref}
            </Typography>
          )}
        </Alert>
      )}

      <TextField
        label={t('settings.monicaImport.connect.urlLabel')}
        placeholder="https://app.monicahq.com"
        value={baseUrl}
        onChange={(e) => {
          setBaseUrl(e.target.value);
          // The address problem is about what was typed; editing it clears it.
          if (connectError?.field === 'url') setConnectError(null);
        }}
        fullWidth
        disabled={!!connectResp}
        error={connectError?.field === 'url'}
        helperText={
          connectError?.field === 'url'
            ? connectError.message
            : t('settings.monicaImport.connect.urlHelp')
        }
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
            onClick={() => void handleConnect()}
            disabled={connecting || !baseUrl.trim() || !apiToken}
            startIcon={connecting ? <CircularProgress size={16} color="inherit" /> : undefined}
          >
            {connecting
              ? t('settings.monicaImport.connect.connecting')
              : t('settings.monicaImport.connect.connect')}
          </Button>
        </Box>
      ) : (
        <Alert severity="info" role="status" sx={{ '& .MuiAlert-message': { width: '100%' } }}>
          <Typography variant="body2" gutterBottom>
            {t('settings.monicaImport.connect.found', {
              contacts: t('settings.monicaImport.connect.foundContacts', {
                count: connectResp.totals.contacts,
              }),
              activities: t('settings.monicaImport.connect.foundActivities', {
                count: connectResp.totals.activities,
              }),
              notes: t('settings.monicaImport.connect.foundNotes', {
                count: connectResp.totals.notes,
              }),
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
