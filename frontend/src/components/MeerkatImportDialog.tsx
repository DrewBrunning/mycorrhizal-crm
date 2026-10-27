import {
  Alert,
  Box,
  Button,
  CircularProgress,
  FormControl,
  FormControlLabel,
  Radio,
  RadioGroup,
  Stack,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  cancelMeerkatImport,
  MEERKAT_IMPORT_BASE,
  type MeerkatConnectResponse,
  type MeerkatUploadResponse,
  startMeerkatFetch,
  uploadMeerkatDatabase,
} from '../api/meerkatImport';
import { useSourceImportWizard } from '../hooks/useSourceImportWizard';
import { getErrorMessage } from '../utils/errorHandler';
import MeerkatLiveConnect from './MeerkatLiveConnect';
import SourceImportWizard from './sourceImport/SourceImportWizard';

interface Props {
  open: boolean;
  onClose: () => void;
  onImportComplete: () => void;
}

const FILE_INPUT_ID = 'meerkat-file-input';

type Mode = 'live' | 'file';

// The Meerkat import assistant (issues #550, #2013). Everything after the
// connect step is the shared SourceImportWizard; this component owns step 0,
// in one of two modes: connect to a running Meerkat over its API (default), or
// upload its .db file and pick which source user to import. Either way it
// hands the wizard a session id. Web-only, per ADR-0007.
export default function MeerkatImportDialog({ open, onClose, onImportComplete }: Props) {
  const { t } = useTranslation();

  const [mode, setMode] = useState<Mode>('live');
  const [live, setLive] = useState<MeerkatConnectResponse | null>(null);
  const [connecting, setConnecting] = useState(false);
  const [acceptUntested, setAcceptUntested] = useState(false);
  const [fileName, setFileName] = useState('');
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [upload, setUpload] = useState<MeerkatUploadResponse | null>(null);
  const [sourceUserId, setSourceUserId] = useState<number | undefined>(undefined);

  // beginFetch runs after upload, so the wizard's startFetch reads the chosen
  // source user through a ref rather than a stale closure.
  const sourceUserRef = useRef<number | undefined>(undefined);
  // Set only for a live connect; the upload path keeps its two-argument call.
  const liveAcceptRef = useRef<boolean | null>(null);

  const startFetch = useCallback(
    (sessionId: string) =>
      liveAcceptRef.current === null
        ? startMeerkatFetch(sessionId, sourceUserRef.current)
        : startMeerkatFetch(sessionId, undefined, liveAcceptRef.current),
    [],
  );
  const wizard = useSourceImportWizard({
    basePath: MEERKAT_IMPORT_BASE,
    startFetch,
  });

  const resetLocal = useCallback(() => {
    setFileName('');
    setUploading(false);
    setUploadError(null);
    setUpload(null);
    setSourceUserId(undefined);
    sourceUserRef.current = undefined;
    liveAcceptRef.current = null;
    setMode('live');
    setLive(null);
    setConnecting(false);
    setAcceptUntested(false);
  }, []);

  const handleClose = useCallback(() => {
    // A connect/upload session the wizard never received (Start not pressed)
    // is invisible to wizard.cancel(), which would leave it -- and a live
    // connect's Meerkat credential -- on the server until it idles out.
    for (const id of [live?.session_id, upload?.session_id]) {
      if (id && id !== wizard.sessionId) void cancelMeerkatImport(id).catch(() => undefined);
    }
    resetLocal();
    onClose();
  }, [live, upload, wizard.sessionId, onClose, resetLocal]);

  const handleComplete = useCallback(() => {
    resetLocal();
    onImportComplete();
  }, [onImportComplete, resetLocal]);

  const handleFile = async (file: File) => {
    setFileName(file.name);
    setUploading(true);
    setUploadError(null);
    try {
      const resp = await uploadMeerkatDatabase(file);
      setUpload(resp);
      setSourceUserId(resp.default_source_user_id);
      sourceUserRef.current = resp.default_source_user_id;
    } catch (err) {
      setUploadError(getErrorMessage(err));
      setFileName('');
    } finally {
      setUploading(false);
    }
  };

  const handleStart = () => {
    if (!upload) return;
    sourceUserRef.current = sourceUserId;
    void wizard.beginFetch(upload.session_id);
  };

  const handleLiveStart = () => {
    if (!live) return;
    liveAcceptRef.current = acceptUntested;
    void wizard.beginFetch(live.session_id);
  };

  const multiUser = (upload?.source_users.length ?? 0) > 1;

  const fileStep = (
    <Stack spacing={2}>
      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
        {t('settings.meerkatImport.connect.description')}
      </Typography>
      {uploadError && <Alert severity="error">{uploadError}</Alert>}

      <Box>
        <input
          id={FILE_INPUT_ID}
          type="file"
          aria-label={t('settings.meerkatImport.connect.chooseFile')}
          accept=".db,.sqlite,.sqlite3"
          style={{ display: 'none' }}
          disabled={!!upload}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void handleFile(f);
          }}
        />
        <Button
          variant="outlined"
          disabled={uploading || !!upload}
          onClick={() => document.getElementById(FILE_INPUT_ID)?.click()}
          startIcon={uploading ? <CircularProgress size={16} color="inherit" /> : undefined}
        >
          {uploading
            ? t('settings.meerkatImport.connect.uploading')
            : t('settings.meerkatImport.connect.chooseFile')}
        </Button>
        {fileName && (
          <Typography variant="caption" sx={{ ml: 1, color: 'text.secondary' }}>
            {fileName}
          </Typography>
        )}
      </Box>
      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
        {t('settings.meerkatImport.connect.fileHelp')}
      </Typography>

      {upload && (
        <Alert severity="info" sx={{ '& .MuiAlert-message': { width: '100%' } }}>
          <Typography variant="body2" gutterBottom>
            {t('settings.meerkatImport.connect.found', {
              contacts: t('settings.meerkatImport.connect.foundContacts', {
                count: upload.totals.contacts,
              }),
              relationships: t('settings.meerkatImport.connect.foundRelationships', {
                count: upload.totals.relationships,
              }),
              notes: t('settings.meerkatImport.connect.foundNotes', {
                count: upload.totals.notes,
              }),
            })}
          </Typography>

          {multiUser && (
            <FormControl sx={{ mt: 1 }}>
              <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                {t('settings.meerkatImport.connect.pickUser')}
              </Typography>
              <RadioGroup
                value={sourceUserId ?? ''}
                onChange={(e) => setSourceUserId(Number(e.target.value))}
              >
                {upload.source_users.map((u) => (
                  <FormControlLabel
                    key={u.id}
                    value={u.id}
                    control={<Radio size="small" />}
                    label={t('settings.meerkatImport.connect.userOption', {
                      name: u.name || u.username || `#${u.id}`,
                      count: u.contacts,
                    })}
                  />
                ))}
              </RadioGroup>
            </FormControl>
          )}

          <Box sx={{ mt: 1 }}>
            <Button variant="contained" onClick={handleStart} disabled={wizard.busy}>
              {t('settings.meerkatImport.connect.start')}
            </Button>
          </Box>
        </Alert>
      )}
    </Stack>
  );

  // The mode is fixed once a session exists, or is being created: switching
  // would strand it.
  const sessionStarted = !!live || !!upload;
  const connectStep = (
    <Stack spacing={2}>
      <Tabs
        value={mode}
        onChange={(_, m: Mode) => setMode(m)}
        aria-label={t('settings.meerkatImport.mode.label')}
      >
        <Tab
          value="live"
          label={t('settings.meerkatImport.mode.live')}
          disabled={connecting || (sessionStarted && mode !== 'live')}
        />
        <Tab
          value="file"
          label={t('settings.meerkatImport.mode.file')}
          disabled={connecting || (sessionStarted && mode !== 'file')}
        />
      </Tabs>
      {mode === 'live' ? (
        <MeerkatLiveConnect
          connected={live}
          onConnected={setLive}
          onConnectingChange={setConnecting}
          acceptUntested={acceptUntested}
          onAcceptUntestedChange={setAcceptUntested}
          onStart={handleLiveStart}
          starting={wizard.busy}
          onUseFile={() => setMode('file')}
        />
      ) : (
        fileStep
      )}
    </Stack>
  );

  return (
    <SourceImportWizard
      open={open}
      onClose={handleClose}
      titleKey="settings.meerkatImport.title"
      sourceLabel="Meerkat"
      wizard={wizard}
      connectStep={connectStep}
      onComplete={handleComplete}
    />
  );
}
