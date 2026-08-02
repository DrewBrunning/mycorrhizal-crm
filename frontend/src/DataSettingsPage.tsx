import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Divider,
  Button,
  Stack,
  Alert,
  CircularProgress,
} from '@mui/material';
import DownloadIcon from '@mui/icons-material/Download';
import TuneIcon from '@mui/icons-material/Tune';
import { exportContacts, exportDataAsCsv, exportContactsAsVcf, ExportFormat, ExportSelection } from './api/export';
import CustomFieldsSettings from './components/CustomFieldsSettings';
import ContactFieldSettings from './components/ContactFieldSettings';
import CalendarSyncSettings from './components/CalendarSyncSettings';
import ExportFieldPickerDialog from './components/ExportFieldPickerDialog';

export default function DataSettingsPage() {
  const { t } = useTranslation();
  const [exporting, setExporting] = useState(false);
  const [exportError, setExportError] = useState('');
  const [exportSuccess, setExportSuccess] = useState('');
  const [exportingVcf, setExportingVcf] = useState(false);
  const [exportVcfError, setExportVcfError] = useState('');
  const [exportVcfSuccess, setExportVcfSuccess] = useState('');
  const [pickerOpen, setPickerOpen] = useState(false);
  const [customExporting, setCustomExporting] = useState(false);
  const [customExportError, setCustomExportError] = useState('');
  const [customExportSuccess, setCustomExportSuccess] = useState('');

  const handleExportData = async () => {
    setExportError('');
    setExportSuccess('');
    setExporting(true);

    try {
      await exportDataAsCsv();
      setExportSuccess(t('settings.export.success'));
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : t('settings.export.error');
      setExportError(errorMessage);
    } finally {
      setExporting(false);
    }
  };

  const handleExportVcf = async () => {
    setExportVcfError('');
    setExportVcfSuccess('');
    setExportingVcf(true);

    try {
      await exportContactsAsVcf();
      setExportVcfSuccess(t('settings.exportVcf.success'));
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : t('settings.exportVcf.error');
      setExportVcfError(errorMessage);
    } finally {
      setExportingVcf(false);
    }
  };

  const handleCustomExport = async (format: ExportFormat, selection: ExportSelection) => {
    setCustomExportError('');
    setCustomExportSuccess('');
    setCustomExporting(true);
    try {
      await exportContacts(format, selection);
      setCustomExportSuccess(t('settings.exportFieldPicker.success'));
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : t('settings.exportFieldPicker.exportError');
      setCustomExportError(errorMessage);
    } finally {
      setCustomExporting(false);
    }
  };

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" gutterBottom sx={{ mb: 1.5 }}>
        {t('settings.data.title')}
      </Typography>

      <ContactFieldSettings />

      <CustomFieldsSettings />

      <CalendarSyncSettings />

      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
            <DownloadIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
            <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>
              {t('settings.export.title')}
            </Typography>
          </Box>
          <Divider sx={{ mb: 1.5 }} />

          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              {t('settings.export.description')}
            </Typography>
            {exportError && <Alert severity="error" sx={{ py: 0 }}>{exportError}</Alert>}
            {exportSuccess && <Alert severity="success" sx={{ py: 0 }}>{exportSuccess}</Alert>}
            <Button
              variant="contained"
              size="small"
              startIcon={exporting ? <CircularProgress size={16} color="inherit" /> : <DownloadIcon />}
              onClick={handleExportData}
              disabled={exporting}
            >
              {exporting ? t('settings.export.exporting') : t('settings.export.downloadButton')}
            </Button>
          </Stack>
        </CardContent>
      </Card>

      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
            <DownloadIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
            <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>
              {t('settings.exportVcf.title')}
            </Typography>
          </Box>
          <Divider sx={{ mb: 1.5 }} />

          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              {t('settings.exportVcf.description')}
            </Typography>
            {exportVcfError && <Alert severity="error" sx={{ py: 0 }}>{exportVcfError}</Alert>}
            {exportVcfSuccess && <Alert severity="success" sx={{ py: 0 }}>{exportVcfSuccess}</Alert>}
            <Stack direction="row" spacing={1}>
              <Button
                variant="contained"
                size="small"
                startIcon={exportingVcf ? <CircularProgress size={16} color="inherit" /> : <DownloadIcon />}
                onClick={handleExportVcf}
                disabled={exportingVcf}
              >
                {exportingVcf ? t('settings.exportVcf.exporting') : t('settings.exportVcf.downloadButton')}
              </Button>
              <Button
                variant="outlined"
                size="small"
                startIcon={<TuneIcon />}
                onClick={() => setPickerOpen(true)}
                disabled={customExporting}
              >
                {t('settings.exportFieldPicker.customExportButton')}
              </Button>
            </Stack>
            {customExportError && <Alert severity="error" sx={{ py: 0 }}>{customExportError}</Alert>}
            {customExportSuccess && <Alert severity="success" sx={{ py: 0 }}>{customExportSuccess}</Alert>}
          </Stack>
        </CardContent>
      </Card>

      <ExportFieldPickerDialog
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        onExport={handleCustomExport}
      />
    </Box>
  );
}
