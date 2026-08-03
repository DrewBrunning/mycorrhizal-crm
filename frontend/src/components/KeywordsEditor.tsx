import { useTranslation } from 'react-i18next';
import { Box, Typography, Chip, TextField, Stack, Button } from '@mui/material';
import { useState } from 'react';
import AddIcon from '@mui/icons-material/Add';

interface KeywordsEditorProps {
  label: string;
  value: string[];
  onChange: (next: string[]) => void;
}

export default function KeywordsEditor({ label, value, onChange }: KeywordsEditorProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState('');

  const addKeyword = () => {
    const trimmed = draft.trim();
    if (trimmed && !value.includes(trimmed)) {
      onChange([...value, trimmed]);
    }
    setDraft('');
  };

  const removeKeyword = (keyword: string) => {
    onChange(value.filter((k) => k !== keyword));
  };

  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {label}
      </Typography>
      <Box sx={{ display: 'flex', gap: 1, mb: 1, flexWrap: 'wrap' }}>
        {value.map((k) => (
          <Chip key={k} label={k} onDelete={() => removeKeyword(k)} size="small" />
        ))}
      </Box>
      <Stack direction="row" spacing={1}>
        <TextField
          size="small"
          fullWidth
          value={draft}
          placeholder={t('contacts.keywords.placeholder')}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              addKeyword();
            }
          }}
        />
        <Button
          size="small"
          variant="outlined"
          startIcon={<AddIcon />}
          onClick={addKeyword}
          disabled={!draft.trim()}
        >
          {t('common.add')}
        </Button>
      </Stack>
    </Box>
  );
}
