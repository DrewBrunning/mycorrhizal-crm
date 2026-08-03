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
  FormControl,
  InputLabel,
  Select,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { CardPersonalInfo } from '../api/contacts';
import { useRowKeys } from '../hooks/useRowKeys';
import { PERSONAL_INFO_KIND_OPTIONS, PERSONAL_INFO_LEVEL_OPTIONS } from '../contactFields';

interface PersonalInfoEditorProps {
  label: string;
  value: CardPersonalInfo[];
  onChange: (next: CardPersonalInfo[]) => void;
}

const EMPTY_INFO: CardPersonalInfo = { kind: 'hobby', value: '' };

export default function PersonalInfoEditor({ label, value, onChange }: PersonalInfoEditorProps) {
  const { t } = useTranslation();
  const rowKeys = useRowKeys(value.length);

  const updateRow = (index: number, patch: Partial<CardPersonalInfo>) => {
    onChange(value.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };

  const removeRow = (index: number) => {
    rowKeys.onRemove(index);
    onChange(value.filter((_, i) => i !== index));
  };

  const addRow = () => {
    rowKeys.onAdd();
    onChange([...value, { ...EMPTY_INFO }]);
  };

  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={1}>
        {value.map((row, index) => (
          <Paper key={rowKeys.keyAt(index)} variant="outlined" sx={{ p: 1.5 }}>
            <Stack spacing={1}>
              <Stack direction="row" spacing={1} alignItems="center">
                <FormControl size="small" sx={{ minWidth: 140 }}>
                  <InputLabel>{t('contacts.personalInfo.kindOptions')}</InputLabel>
                  <Select
                    label={t('contacts.personalInfo.kindOptions')}
                    value={row.kind}
                    onChange={(e) => updateRow(index, { kind: e.target.value })}
                  >
                    {PERSONAL_INFO_KIND_OPTIONS.map((opt) => (
                      <MenuItem key={opt} value={opt}>
                        {t(`contacts.personalInfo.kindOptions.${opt}`)}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <TextField
                  label={t('contacts.personalInfo.value')}
                  size="small"
                  fullWidth
                  value={row.value}
                  onChange={(e) => updateRow(index, { value: e.target.value })}
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
              <Stack direction="row" spacing={1}>
                <FormControl size="small" sx={{ minWidth: 140 }}>
                  <InputLabel>{t('contacts.personalInfo.levelOptions')}</InputLabel>
                  <Select
                    label={t('contacts.personalInfo.levelOptions')}
                    value={row.level || ''}
                    onChange={(e) => updateRow(index, { level: e.target.value })}
                  >
                    <MenuItem value="">
                      <em>{t('contacts.personalInfo.levelNone')}</em>
                    </MenuItem>
                    {PERSONAL_INFO_LEVEL_OPTIONS.map((opt) => (
                      <MenuItem key={opt} value={opt}>
                        {t(`contacts.personalInfo.levelOptions.${opt}`)}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <TextField
                  label={t('contacts.personalInfo.label')}
                  size="small"
                  fullWidth
                  value={row.label || ''}
                  onChange={(e) => updateRow(index, { label: e.target.value })}
                />
              </Stack>
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
