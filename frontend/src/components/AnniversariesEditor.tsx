import { useTranslation } from 'react-i18next';
import {
  Box,
  Typography,
  Stack,
  TextField,
  IconButton,
  Button,
  Paper,
  MenuItem,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { CardAnniversary, parseAnniversaryDate } from '../api/contacts';
import { useRowKeys } from '../hooks/useRowKeys';

interface AnniversariesEditorProps {
  label: string;
  value: CardAnniversary[];
  onChange: (next: CardAnniversary[]) => void;
}

const EMPTY_ANNIVERSARY: CardAnniversary = { kind: 'wedding', date: {} };

export default function AnniversariesEditor({ label, value, onChange }: AnniversariesEditorProps) {
  const { t } = useTranslation();
  const rowKeys = useRowKeys(value.length);

  const updateRow = (index: number, patch: Partial<CardAnniversary>) => {
    onChange(value.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };

  const removeRow = (index: number) => {
    rowKeys.onRemove(index);
    onChange(value.filter((_, i) => i !== index));
  };

  const addRow = () => {
    rowKeys.onAdd();
    onChange([...value, { ...EMPTY_ANNIVERSARY }]);
  };

  const formatForEdit = (a: CardAnniversary): string => {
    if (a.date.partial) {
      const { year, month, day } = a.date.partial;
      const mm = month != null ? String(month).padStart(2, '0') : '';
      const dd = day != null ? String(day).padStart(2, '0') : '';
      if (mm && dd) return year != null ? `${year}-${mm}-${dd}` : `--${mm}-${dd}`;
    }
    if (a.date.timestamp) return a.date.timestamp.slice(0, 10);
    return '';
  };

  return (
    <Box>
      <Typography variant="subtitle2" component="p" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={1}>
        {value.map((row, index) => (
          <Paper key={rowKeys.keyAt(index)} variant="outlined" sx={{ p: 1.5 }}>
            <Stack direction="row" spacing={1} alignItems="center">
              <TextField
                select
                label={t('contacts.anniversaryFields.kindLabel')}
                size="small"
                value={row.kind}
                onChange={(e) => updateRow(index, { kind: e.target.value as CardAnniversary['kind'] })}
              >
                <MenuItem value="birth">{t('contacts.anniversaryFields.kindOptions.birth')}</MenuItem>
                <MenuItem value="death">{t('contacts.anniversaryFields.kindOptions.death')}</MenuItem>
                <MenuItem value="wedding">{t('contacts.anniversaryFields.kindOptions.wedding')}</MenuItem>
              </TextField>
              <TextField
                label={t('contacts.anniversaryFields.date')}
                size="small"
                fullWidth
                value={formatForEdit(row)}
                placeholder="--MM-DD"
                onChange={(e) => updateRow(index, { date: parseAnniversaryDate(e.target.value) })}
              />
              <IconButton
                size="small"
                color="error"
                onClick={() => removeRow(index)}
                aria-label={t('common.delete')}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Stack>
            {row.place && (row.place.components?.length || row.place.full) && (
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                {row.place.full || (row.place.components || []).map((c) => c.value).filter(Boolean).join(', ')}
              </Typography>
            )}
          </Paper>
        ))}
        <Box>
          <Button size="small" startIcon={<AddIcon />} onClick={addRow} variant="outlined">
            {t('common.add')}
          </Button>
        </Box>
      </Stack>
    </Box>
  );
}
