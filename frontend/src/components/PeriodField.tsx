import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import EditIcon from '@mui/icons-material/Edit';
import { Box, IconButton, Stack, TextField, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { CardTemporalRange } from '../api/contacts';

interface PeriodFieldProps {
  label: string;
  range?: CardTemporalRange;
  // Persist the new range; undefined clears the period. Resolves once the
  // update has been applied so the row can leave edit mode.
  onSave: (range: CardTemporalRange | undefined) => Promise<void>;
}

// ADR 0025: a self-contained inline editor for one entry's start/end period,
// used by the professional (organization/title) section. Whole years only,
// matching AddressFields' period inputs; either side may be left blank
// (open-ended). A blank/blank save clears the period. It owns its own
// edit/cancel/save state so ContactInformation's scalar-field edit machine is
// untouched.
export default function PeriodField({ label, range, onSave }: PeriodFieldProps) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [startYear, setStartYear] = useState('');
  const [endYear, setEndYear] = useState('');

  const startDisplay = range?.start?.year != null ? String(range.start.year) : '';
  const endDisplay = range?.end?.year != null ? String(range.end.year) : '';

  useEffect(() => {
    if (editing) return;
    setStartYear(startDisplay);
    setEndYear(endDisplay);
  }, [editing, startDisplay, endDisplay]);

  const begin = () => {
    setStartYear(startDisplay);
    setEndYear(endDisplay);
    setEditing(true);
  };

  const commit = async () => {
    setSaving(true);
    try {
      const start = startYear ? Number.parseInt(startYear, 10) : undefined;
      const end = endYear ? Number.parseInt(endYear, 10) : undefined;
      if ((startYear && Number.isNaN(start)) || (endYear && Number.isNaN(end))) return;
      await onSave(
        start == null && end == null
          ? undefined
          : {
              ...(start != null ? { start: { year: start } } : {}),
              ...(end != null ? { end: { year: end } } : {}),
            },
      );
      setEditing(false);
    } finally {
      setSaving(false);
    }
  };

  const display =
    startDisplay && endDisplay
      ? `${startDisplay} – ${endDisplay}`
      : startDisplay
        ? t('contactDetail.period.since', { year: startDisplay })
        : endDisplay
          ? t('contactDetail.period.until', { year: endDisplay })
          : '';

  return (
    <Box sx={{ '&:hover .period-edit-icon, &:focus-within .period-edit-icon': { opacity: 1 } }}>
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <Typography
          variant="caption"
          sx={{ color: 'text.secondary', fontFamily: '"IBM Plex Mono", monospace' }}
        >
          {label}
        </Typography>
        {!editing && (
          <IconButton
            className="period-edit-icon"
            size="small"
            color="primary"
            onClick={begin}
            aria-label={t('common.edit')}
            data-testid="period-edit"
            sx={{ ml: 0.5, p: 0.25, opacity: display ? 0 : 1, minWidth: 24, minHeight: 24 }}
          >
            <EditIcon sx={{ fontSize: 18 }} />
          </IconButton>
        )}
      </Box>
      {editing ? (
        <Stack direction="row" spacing={1} sx={{ mt: 0.5, alignItems: 'center' }}>
          <TextField
            type="number"
            size="small"
            fullWidth
            label={t('contactDetail.period.from')}
            value={startYear}
            onChange={(e) => setStartYear(e.target.value)}
            slotProps={{ htmlInput: { min: 1900, max: 2100, 'data-testid': 'period-start' } }}
          />
          <TextField
            type="number"
            size="small"
            fullWidth
            label={t('contactDetail.period.to')}
            value={endYear}
            onChange={(e) => setEndYear(e.target.value)}
            slotProps={{ htmlInput: { min: 1900, max: 2100, 'data-testid': 'period-end' } }}
          />
          <IconButton
            size="small"
            color="primary"
            onClick={commit}
            disabled={saving}
            aria-label={t('common.save')}
            data-testid="period-save"
          >
            <CheckIcon fontSize="small" />
          </IconButton>
          <IconButton
            size="small"
            onClick={() => setEditing(false)}
            aria-label={t('common.cancel')}
          >
            <CloseIcon fontSize="small" />
          </IconButton>
        </Stack>
      ) : display ? (
        <Typography variant="body1">{display}</Typography>
      ) : (
        <Typography variant="body1" color="text.secondary">
          -
        </Typography>
      )}
    </Box>
  );
}
