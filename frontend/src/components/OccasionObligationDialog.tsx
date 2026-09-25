import {
  Autocomplete,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  OCCASION_OBLIGATION_KINDS,
  type OccasionObligation,
  type OccasionObligationInput,
  type OccasionSensitivity,
} from '../api/occasionObligations';
import AppDialog from './AppDialog';

export interface OccasionObligationFormData {
  kind: string;
  label: string;
  anchorMonth: number | null;
  anchorDay: number | null;
  leadTimeDays: number;
  active: boolean;
  sensitivity: OccasionSensitivity;
  notes?: string;
}

interface OccasionObligationDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (data: OccasionObligationFormData) => Promise<void>;
  obligation?: OccasionObligation | null;
}

const MONTHS = Array.from({ length: 12 }, (_, i) => i + 1);
const DAYS = Array.from({ length: 31 }, (_, i) => i + 1);

export default function OccasionObligationDialog({
  open,
  onClose,
  onSave,
  obligation,
}: OccasionObligationDialogProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState('');
  const [label, setLabel] = useState('');
  const [anchorMonth, setAnchorMonth] = useState<number | ''>('');
  const [anchorDay, setAnchorDay] = useState<number | ''>('');
  const [leadTimeDays, setLeadTimeDays] = useState('0');
  const [active, setActive] = useState(true);
  const [sensitivity, setSensitivity] = useState<OccasionSensitivity>('normal');
  const [notes, setNotes] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setKind(obligation?.kind || 'card');
      setLabel(obligation?.label || '');
      setAnchorMonth(obligation?.anchor_month ?? '');
      setAnchorDay(obligation?.anchor_day ?? '');
      setLeadTimeDays(obligation ? String(obligation.lead_time_days) : '0');
      setActive(obligation ? obligation.active : true);
      setSensitivity(obligation?.sensitivity || 'normal');
      setNotes(obligation?.notes || '');
      setError('');
    }
  }, [open, obligation]);

  const handleSave = async () => {
    const trimmedLabel = label.trim();
    if (!trimmedLabel) {
      setError(t('occasions.obligation.validation.labelRequired'));
      return;
    }
    // Mirrors the backend's "both or neither" anchor rule (ADR 0024) client-side.
    if ((anchorMonth === '') !== (anchorDay === '')) {
      setError(t('occasions.obligation.validation.anchorBothOrNeither'));
      return;
    }
    const leadTime = leadTimeDays.trim() === '' ? 0 : parseInt(leadTimeDays, 10);
    if (Number.isNaN(leadTime) || leadTime < 0) {
      setError(t('occasions.obligation.validation.invalidLeadTime'));
      return;
    }

    setSaving(true);
    try {
      await onSave({
        kind: kind.trim() || 'card',
        label: trimmedLabel,
        anchorMonth: anchorMonth === '' ? null : anchorMonth,
        anchorDay: anchorDay === '' ? null : anchorDay,
        leadTimeDays: leadTime,
        active,
        sensitivity,
        notes: notes.trim() || undefined,
      });
      onClose();
    } catch {
      setError(t('occasions.obligation.validation.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {obligation ? t('occasions.obligation.editTitle') : t('occasions.obligation.createTitle')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Autocomplete
            freeSolo
            options={[...OCCASION_OBLIGATION_KINDS]}
            value={kind}
            onInputChange={(_e, value) => {
              setKind(value);
              setError('');
            }}
            renderInput={(params) => (
              <TextField
                {...params}
                label={t('occasions.obligation.kind')}
                helperText={t('occasions.obligation.kindHint')}
              />
            )}
          />

          <TextField
            label={t('occasions.obligation.label')}
            value={label}
            onChange={(e) => {
              setLabel(e.target.value);
              setError('');
            }}
            fullWidth
            required
            helperText={t('occasions.obligation.labelHint')}
          />

          <Box sx={{ display: 'flex', gap: 1 }}>
            <TextField
              select
              label={t('occasions.obligation.anchorMonth')}
              value={anchorMonth}
              onChange={(e) => {
                setAnchorMonth(e.target.value === '' ? '' : Number(e.target.value));
                setError('');
              }}
              fullWidth
            >
              <MenuItem value="">{t('occasions.obligation.anchorDateUnset')}</MenuItem>
              {MONTHS.map((m) => (
                <MenuItem key={m} value={m}>
                  {t(`occasions.months.${m}`)}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              select
              label={t('occasions.obligation.anchorDay')}
              value={anchorDay}
              onChange={(e) => {
                setAnchorDay(e.target.value === '' ? '' : Number(e.target.value));
                setError('');
              }}
              fullWidth
            >
              <MenuItem value="">{t('occasions.obligation.anchorDateUnset')}</MenuItem>
              {DAYS.map((d) => (
                <MenuItem key={d} value={d}>
                  {d}
                </MenuItem>
              ))}
            </TextField>
          </Box>

          <TextField
            label={t('occasions.obligation.leadTimeDays')}
            type="number"
            value={leadTimeDays}
            onChange={(e) => setLeadTimeDays(e.target.value)}
            fullWidth
            helperText={t('occasions.obligation.leadTimeDaysHint')}
            slotProps={{ htmlInput: { min: 0, max: 365 } }}
          />

          <FormControlLabel
            control={<Switch checked={active} onChange={(e) => setActive(e.target.checked)} />}
            label={t('occasions.obligation.active')}
          />

          <TextField
            select
            label={t('occasions.obligation.sensitivity')}
            value={sensitivity}
            onChange={(e) => setSensitivity(e.target.value as OccasionSensitivity)}
            fullWidth
          >
            <MenuItem value="normal">{t('occasions.obligation.sensitivities.normal')}</MenuItem>
            <MenuItem value="private">{t('occasions.obligation.sensitivities.private')}</MenuItem>
            <MenuItem value="secret">{t('occasions.obligation.sensitivities.secret')}</MenuItem>
          </TextField>

          <TextField
            label={t('occasions.obligation.notes')}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            fullWidth
            multiline
            minRows={2}
          />

          {error && (
            <Typography color="error" variant="body2">
              {error}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleSave()} variant="contained" disabled={saving}>
          {t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}

// Rebuilds an OccasionObligationInput for the wire from a form payload + the
// target entity.
export function toOccasionObligationInput(
  entityId: string,
  data: OccasionObligationFormData,
): OccasionObligationInput {
  return {
    entity_id: entityId,
    kind: data.kind,
    label: data.label,
    anchor_month: data.anchorMonth,
    anchor_day: data.anchorDay,
    lead_time_days: data.leadTimeDays,
    active: data.active,
    sensitivity: data.sensitivity,
    notes: data.notes,
  };
}
