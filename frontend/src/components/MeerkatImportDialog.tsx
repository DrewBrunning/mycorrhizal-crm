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
  Typography,
} from '@mui/material';
import { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  MEERKAT_IMPORT_BASE,
  type MeerkatUploadResponse,
  startMeerkatFetch,
  uploadMeerkatDatabase,
} from '../api/meerkatImport';
import { useSourceImportWizard } from '../hooks/useSourceImportWizard';
import { getErrorMessage } from '../utils/errorHandler';
import SourceImportWizard from './sourceImport/SourceImportWizard';

interface Props {
  open: boolean;
  onClose: () => void;
  onImportComplete: () => void;
}

const FILE_INPUT_ID = 'meerkat-file-input';

// The Meerkat import assistant (issue #550). Everything after the connect
// step is the shared SourceImportWizard; this component owns step 0 — upload
// the .db file, then pick which source user to import — and hands the wizard
// a session id. Web-only, direct-DB per ADR-0007.
export default function MeerkatImportDialog({ open, onClose, onImportComplete }: Props) {
  const { t } = useTranslation();

  const [fileName, setFileName] = useState('');
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [upload, setUpload] = useState<MeerkatUploadResponse | null>(null);
  const [sourceUserId, setSourceUserId] = useState<number | undefined>(undefined);

  // beginFetch runs after upload, so the wizard's startFetch reads the chosen
  // source user through a ref rather than a stale closure.
  const sourceUserRef = useRef<number | undefined>(undefined);

  const startFetch = useCallback(
    (sessionId: string) => startMeerkatFetch(sessionId, sourceUserRef.current),
    [],
  );
  const wizard = useSourceImportWizard({ basePath: MEERKAT_IMPORT_BASE, startFetch });

  const resetLocal = useCallback(() => {
    setFileName('');
    setUploading(false);
    setUploadError(null);
    setUpload(null);
    setSourceUserId(undefined);
    sourceUserRef.current = undefined;
  }, []);

  const handleClose = useCallback(() => {
    resetLocal();
    onClose();
  }, [onClose, resetLocal]);

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

  const multiUser = (upload?.source_users.length ?? 0) > 1;

  const connectStep = (
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
              contacts: upload.totals.contacts,
              relationships: upload.totals.relationships,
              notes: upload.totals.notes,
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
