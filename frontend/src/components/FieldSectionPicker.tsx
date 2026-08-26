import { mdiLockOpenVariantOutline } from '@mdi/js';
import LockOutlined from '@mui/icons-material/LockOutlined';
import {
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormLabel,
  SvgIcon,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { EXPORT_FIELD_SECTIONS } from '../api/export';

interface FieldSectionPickerProps {
  selected: Set<string>;
  onToggle: (token: string, checked: boolean) => void;
  sensitiveRevealed: boolean;
  onReveal: () => void;
}

// FieldSectionPicker is the T9 "choose which fields" checkbox list
// (Google-Contacts-style coarse-grained picker), extracted out of
// ExportFieldPickerDialog so P1 contact sharing's ShareContactDialog can
// reuse it exactly rather than reimplementing the sensitivity foot-gun guard
// (T9 explicitly calls this
// picker out as meant to be reused by sharing). Ordinary sections are
// opt-out (checked by default, owned by the caller via `selected`);
// sensitivity-marked sections are opt-in AND gated: disabled, visually
// flagged, and only interactive after the deliberate "reveal" confirmation
// below -- an unchecked box alone is not enough (second
// clarification).
export default function FieldSectionPicker({
  selected,
  onToggle,
  sensitiveRevealed,
  onReveal,
}: FieldSectionPickerProps) {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const handleRevealConfirm = () => {
    onReveal();
    setConfirmOpen(false);
  };

  return (
    <>
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
                      onChange={(e) => onToggle(section.token, e.target.checked)}
                    />
                  }
                  label={
                    <Box component="span" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      {t(`settings.exportFieldPicker.sections.${section.token}`)}
                      {isSensitive && !locked && (
                        <SvgIcon
                          fontSize="small"
                          sx={{ color: 'success.main', verticalAlign: 'middle' }}
                        >
                          <path d={mdiLockOpenVariantOutline} />
                        </SvgIcon>
                      )}
                    </Box>
                  }
                />
                {locked && (
                  <LockOutlined
                    fontSize="inherit"
                    color="disabled"
                    sx={{ ml: 0.5, opacity: 0.5 }}
                  />
                )}
              </Box>
            );
          })}
        </Box>
      </FormControl>

      {!sensitiveRevealed ? (
        <Button
          variant="outlined"
          size="small"
          startIcon={<LockOutlined />}
          onClick={() => setConfirmOpen(true)}
          sx={{ alignSelf: 'flex-start' }}
        >
          {t('settings.exportFieldPicker.revealButton')}
        </Button>
      ) : (
        <Typography
          variant="body2"
          color="text.secondary"
          sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
        >
          <SvgIcon fontSize="small" color="success">
            <path d={mdiLockOpenVariantOutline} />
          </SvgIcon>
          {t('settings.exportFieldPicker.revealedHint')}
        </Typography>
      )}

      <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)} maxWidth="xs">
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <LockOutlined color="action" />
          {t('settings.exportFieldPicker.revealTitle')}
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2">{t('settings.exportFieldPicker.revealConfirm')}</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmOpen(false)}>
            {t('settings.exportFieldPicker.revealCancel')}
          </Button>
          <Button onClick={handleRevealConfirm} variant="contained">
            {t('settings.exportFieldPicker.revealConfirmButton')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
