import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import { Box, Button, IconButton, Paper, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { CardNote } from '../api/contacts';
import { useRowKeys } from '../hooks/useRowKeys';

interface CardNotesEditorProps {
  label: string;
  value: CardNote[];
  onChange: (next: CardNote[]) => void;
}

export default function CardNotesEditor({ label, value, onChange }: CardNotesEditorProps) {
  const { t } = useTranslation();
  const rowKeys = useRowKeys(value.length);

  const updateRow = (index: number, patch: Partial<CardNote>) => {
    onChange(value.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };

  const removeRow = (index: number) => {
    rowKeys.onRemove(index);
    onChange(value.filter((_, i) => i !== index));
  };

  const addRow = () => {
    rowKeys.onAdd();
    onChange([...value, { note: '' }]);
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
              <Stack direction="row" spacing={1} alignItems="flex-start">
                <TextField
                  label={t('contacts.cardNotes.note')}
                  size="small"
                  fullWidth
                  multiline
                  minRows={2}
                  value={row.note}
                  onChange={(e) => updateRow(index, { note: e.target.value })}
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
              {(row.author?.name || row.author?.uri || row.created?.utc) && (
                <Typography variant="caption" color="text.secondary">
                  {row.author?.name && `${row.author.name} · `}
                  {row.author?.uri && `${row.author.uri} · `}
                  {row.created?.utc && t('contacts.cardNotes.createdOn', { date: row.created.utc })}
                </Typography>
              )}
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
