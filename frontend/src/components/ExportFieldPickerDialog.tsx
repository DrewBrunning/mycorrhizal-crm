import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Checkbox,
  FormControlLabel,
  FormControl,
  FormLabel,
  RadioGroup,
  Radio,
  Alert,
  Dialog,
} from '@mui/material';
import LockOutlined from '@mui/icons-material/LockOutlined';
import WarningAmber from '@mui/icons-material/WarningAmber';
import AppDialog from './AppDialog';
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

// ExportFieldPickerDialog is the WP-97/T9 "choose which fields to export"
// dialog (Google-Contacts-style coarse-grained picker). Ordinary sections are
// opt-out (checked by default); sensitivity-marked sections (§91.13) are
// opt-in AND gated: their controls are disabled, visually flagged with a
// warning, and only become interactive after the explicit "reveal sensitive
// fields" action (foot-gun prevention, docs/fork-plan/92-delivery-roadmap.md
// §92.6b's second clarification — an unchecked box alone is not enough).
export default function ExportFieldPickerDialog({
  open,
  onClose,
  onExport,
}: ExportFieldPickerDialogProps) {
  const { t } = useTranslation();
  const [format, setFormat] = useState<ExportFormat>('vcf4');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [sensitiveRevealed, setSensitiveRevealed] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
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

  const handleRevealConfirm = () => {
    setSensitiveRevealed(true);
    setConfirmOpen(false);
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
    <>
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

            <FormControl component="fieldset">
              <FormLabel component="legend">{t('settings.exportFieldPicker.fields')}</FormLabel>
              <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                {EXPORT_FIELD_SECTIONS.map((section) => {
                  const isSensitive = section.sensitive;
                  const locked = isSensitive && !sensitiveRevealed;
                  const checked = selected.has(section.token);
                  return (
                    <Box key={section.token} sx={{ display: 'flex', alignItems: 'center' }}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={locked ? false : checked}
                            disabled={locked}
                            onChange={(e) => toggle(section.token, e.target.checked)}
                          />
                        }
                        label={
                          <Box component="span" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            {t(`settings.exportFieldPicker.sections.${section.token}`)}
                            {isSensitive && (
                              <WarningAmber
                                fontSize="small"
                                color="warning"
                                sx={{ verticalAlign: 'middle' }}
                              />
                            )}
                          </Box>
                        }
                      />
                      {locked && (
                        <LockOutlined fontSize="small" color="action" sx={{ ml: 0.5 }} />
                      )}
                    </Box>
                  );
                })}
              </Box>
            </FormControl>

            {!sensitiveRevealed ? (
              <Button
                variant="outlined"
                color="warning"
                size="small"
                startIcon={<LockOutlined />}
                onClick={() => setConfirmOpen(true)}
                sx={{ alignSelf: 'flex-start' }}
              >
                {t('settings.exportFieldPicker.revealButton')}
              </Button>
            ) : (
              <Alert severity="warning" sx={{ py: 0.5 }}>
                {t('settings.exportFieldPicker.revealedHint')}
              </Alert>
            )}

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

      <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)} maxWidth="xs">
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <WarningAmber color="warning" />
          {t('settings.exportFieldPicker.revealTitle')}
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            {t('settings.exportFieldPicker.revealConfirm')}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmOpen(false)}>
            {t('settings.exportFieldPicker.revealCancel')}
          </Button>
          <Button onClick={handleRevealConfirm} variant="contained" color="warning">
            {t('settings.exportFieldPicker.revealConfirmButton')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
