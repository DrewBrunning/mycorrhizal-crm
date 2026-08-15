import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  FormControl,
  FormLabel,
  RadioGroup,
  Radio,
  FormControlLabel,
  Alert,
} from '@mui/material';
import AppDialog from './AppDialog';
import FieldSectionPicker from './FieldSectionPicker';
import {
  ExportFormat,
  EXPORT_FIELD_SECTIONS,
  ExportSelection,
} from '../api/export';

interface ExportFieldPickerDialogProps {
  open: boolean;
  onClose: () => void;
  onExport: (format: ExportFormat, selection: ExportSelection) => Promise<void>;
}

// ExportFieldPickerDialog is the T9 "choose which fields to export"
// dialog (Google-Contacts-style coarse-grained picker). Ordinary sections are
// opt-out (checked by default); sensitivity-marked sections  are
// opt-in AND gated: their controls are disabled, visually flagged with a
// warning, and only become interactive after the explicit "reveal sensitive
// fields" action (foot-gun prevention — an unchecked box alone is not enough).
export default function ExportFieldPickerDialog({
  open,
  onClose,
  onExport,
}: ExportFieldPickerDialogProps) {
  const { t } = useTranslation();
  const [format, setFormat] = useState<ExportFormat>('vcf4');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sensitiveRevealed, setSensitiveRevealed] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState('');

  const defaultSelection = useMemo(
    () => new Set(EXPORT_FIELD_SECTIONS.filter((s) => !s.sensitive).map((s) => s.token)),
    []
  );

  useEffect(() => {
    if (open) {
      setSelected(new Set(defaultSelection));
      setSensitiveRevealed(false);
      setFormat('vcf4');
      setError('');
    }
  }, [open, defaultSelection]);

  const toggle = (token: string, checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (checked) {
        next.add(token);
      } else {
        next.delete(token);
      }
      return next;
    });
  };

  const handleExport = async () => {
    const includeSensitive =
      sensitiveRevealed &&
      EXPORT_FIELD_SECTIONS.some((s) => s.sensitive && selected.has(s.token));

    setExporting(true);
    setError('');
    try {
      await onExport(format, {
        sections: [...selected],
        includeSensitive,
      });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.exportFieldPicker.exportError'));
    } finally {
      setExporting(false);
    }
  };

  const handleClose = () => {
    if (exporting) return;
    onClose();
  };

  return (
      <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
        <DialogTitle>{t('settings.exportFieldPicker.title')}</DialogTitle>
        <DialogContent>
          <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Typography variant="body2" color="text.secondary">
              {t('settings.exportFieldPicker.description')}
            </Typography>

            <FormControl component="fieldset">
              <FormLabel component="legend">{t('settings.exportFieldPicker.format')}</FormLabel>
              <RadioGroup
                row
                value={format}
                onChange={(e) => setFormat(e.target.value as ExportFormat)}
              >
                <FormControlLabel value="vcf4" control={<Radio />} label={t('settings.exportFieldPicker.formats.vcf4')} />
                <FormControlLabel value="vcf3" control={<Radio />} label={t('settings.exportFieldPicker.formats.vcf3')} />
                <FormControlLabel value="jscontact" control={<Radio />} label={t('settings.exportFieldPicker.formats.jscontact')} />
              </RadioGroup>
            </FormControl>

            <FieldSectionPicker
              selected={selected}
              onToggle={toggle}
              sensitiveRevealed={sensitiveRevealed}
              onReveal={() => setSensitiveRevealed(true)}
            />

            {error && <Alert severity="error" sx={{ py: 0 }}>{error}</Alert>}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} disabled={exporting}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleExport} variant="contained" disabled={exporting || selected.size === 0}>
            {exporting ? t('settings.exportFieldPicker.exporting') : t('settings.exportFieldPicker.exportButton')}
          </Button>
        </DialogActions>
      </AppDialog>
  );
}
