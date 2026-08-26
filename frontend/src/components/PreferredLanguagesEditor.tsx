import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import {
  Autocomplete,
  Box,
  Button,
  IconButton,
  Paper,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { CardLanguagePref } from '../api/contacts';
import { CONTEXT_OPTIONS } from '../contactFields';
import { useRowKeys } from '../hooks/useRowKeys';

interface PreferredLanguagesEditorProps {
  label: string;
  value: CardLanguagePref[];
  onChange: (next: CardLanguagePref[]) => void;
}

export default function PreferredLanguagesEditor({
  label,
  value,
  onChange,
}: PreferredLanguagesEditorProps) {
  const { t } = useTranslation();
  const rowKeys = useRowKeys(value.length);

  const updateRow = (index: number, patch: Partial<CardLanguagePref>) => {
    onChange(value.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };

  const removeRow = (index: number) => {
    rowKeys.onRemove(index);
    onChange(value.filter((_, i) => i !== index));
  };

  const addRow = () => {
    rowKeys.onAdd();
    onChange([...value, { language: '' }]);
  };

  return (
    <Box>
      <Typography variant="subtitle2" component="p" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={1}>
        {value.map((row, index) => (
          <Paper key={rowKeys.keyAt(index)} variant="outlined" sx={{ p: 1.5 }}>
            <Stack spacing={1}>
              <Stack direction="row" spacing={1} alignItems="center">
                <TextField
                  label={t('contacts.preferredLanguages.language')}
                  size="small"
                  fullWidth
                  placeholder="en / fr-CA"
                  value={row.language}
                  onChange={(e) => updateRow(index, { language: e.target.value })}
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
              <Autocomplete
                multiple
                freeSolo
                size="small"
                options={CONTEXT_OPTIONS as readonly string[]}
                value={row.contexts || []}
                onChange={(_, newValue) => updateRow(index, { contexts: newValue as string[] })}
                getOptionLabel={(opt) => t(`contacts.contexts.${opt}`, opt)}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label={t('contacts.preferredLanguages.contexts')}
                    size="small"
                  />
                )}
              />
            </Stack>
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
