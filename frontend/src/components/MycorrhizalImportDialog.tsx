import { Alert, Box, Button, CircularProgress, Stack, Typography } from '@mui/material';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  MYCORRHIZAL_IMPORT_BASE,
  type MycorrhizalUploadResponse,
  startMycorrhizalFetch,
  uploadMycorrhizalBundle,
} from '../api/mycorrhizalImport';
import { useSourceImportWizard } from '../hooks/useSourceImportWizard';
import { getErrorMessage } from '../utils/errorHandler';
import SourceImportWizard from './sourceImport/SourceImportWizard';

interface Props {
  open: boolean;
  onClose: () => void;
  onImportComplete: () => void;
}

const FILE_INPUT_ID = 'mycorrhizal-file-input';

// The Mycorrhizal account-bundle import source (issue #1260, ADR 0028
// Decision 3). Everything after the connect step is the shared
// SourceImportWizard; this component owns step 0 — upload a bundle produced by
// GET /export/account. Web-only.
export default function MycorrhizalImportDialog({ open, onClose, onImportComplete }: Props) {
  const { t } = useTranslation();

  const [fileName, setFileName] = useState('');
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [upload, setUpload] = useState<MycorrhizalUploadResponse | null>(null);

  const startFetch = useCallback((sessionId: string) => startMycorrhizalFetch(sessionId), []);
  const wizard = useSourceImportWizard({ basePath: MYCORRHIZAL_IMPORT_BASE, startFetch });

  const resetLocal = useCallback(() => {
    setFileName('');
    setUploading(false);
    setUploadError(null);
    setUpload(null);
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
      const resp = await uploadMycorrhizalBundle(file);
      setUpload(resp);
    } catch (err) {
      setUploadError(getErrorMessage(err));
      setFileName('');
    } finally {
      setUploading(false);
    }
  };

  const handleStart = () => {
    if (!upload) return;
    void wizard.beginFetch(upload.session_id);
  };

  const connectStep = (
    <Stack spacing={2}>
      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
        {t('settings.mycorrhizalImport.connect.description')}
      </Typography>
      {uploadError && <Alert severity="error">{uploadError}</Alert>}

      <Box>
        <input
          id={FILE_INPUT_ID}
          type="file"
          aria-label={t('settings.mycorrhizalImport.connect.chooseFile')}
          accept=".json,application/json"
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
            ? t('settings.mycorrhizalImport.connect.uploading')
            : t('settings.mycorrhizalImport.connect.chooseFile')}
        </Button>
        {fileName && (
          <Typography variant="caption" sx={{ ml: 1, color: 'text.secondary' }}>
            {fileName}
          </Typography>
        )}
      </Box>
      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
        {t('settings.mycorrhizalImport.connect.fileHelp')}
      </Typography>

      {upload && (
        <Alert severity="info">
          <Typography variant="body2" gutterBottom>
            {t('settings.mycorrhizalImport.connect.found', {
              contacts: upload.totals.contacts,
              relationships: upload.totals.relationships,
              notes: upload.totals.notes,
            })}
          </Typography>
          <Box sx={{ mt: 1 }}>
            <Button variant="contained" onClick={handleStart} disabled={wizard.busy}>
              {t('settings.mycorrhizalImport.connect.start')}
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
      titleKey="settings.mycorrhizalImport.title"
      sourceLabel="Mycorrhizal"
      wizard={wizard}
      connectStep={connectStep}
      onComplete={handleComplete}
    />
  );
}
