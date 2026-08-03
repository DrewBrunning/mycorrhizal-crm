import { useTranslation } from 'react-i18next';
import {
  Box,
  Typography,
  Stack,
  TextField,
  IconButton,
  Button,
  Paper,
  Autocomplete,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { CardOnlineService, onlineServicesToRows, rowsToOnlineServices, OnlineServiceRow } from '../api/contacts';
import { useRowKeys } from '../hooks/useRowKeys';
import { CONTEXT_OPTIONS } from '../contactFields';

interface OnlineServiceEditorProps {
  label: string;
  value: CardOnlineService[];
  onChange: (next: CardOnlineService[]) => void;
  /** When true, the service-name field is editable (WP3's social/other services). */
  showService?: boolean;
  /** When true, the URI field is the only value input (IMPP-style). */
  uriOnly?: boolean;
}

const EMPTY_ROW: OnlineServiceRow = {
  service: '',
  uri: '',
  user: '',
  label: '',
  contexts: [],
};
export default function OnlineServiceEditor({
  label,
  value,
  onChange,
  showService = true,
  uriOnly = false,
}: OnlineServiceEditorProps) {
  const { t } = useTranslation();
  const rowKeys = useRowKeys(value.length);
  const rows = onlineServicesToRows(value);

  const updateRow = (index: number, patch: Partial<OnlineServiceRow>) => {
    const next = rows.map((r, i) => (i === index ? { ...r, ...patch } : r));
    onChange(rowsToOnlineServices(next));
  };

  const removeRow = (index: number) => {
    rowKeys.onRemove(index);
    onChange(rowsToOnlineServices(rows.filter((_, i) => i !== index)));
  };

  const addRow = () => {
    rowKeys.onAdd();
    onChange([...value, { ...EMPTY_ROW }]);
  };

  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={1}>
        {rows.map((row, index) => (
          <Paper key={rowKeys.keyAt(index)} variant="outlined" sx={{ p: 1.5 }}>
            <Stack spacing={1}>
              {showService && (
                <TextField
                  label={t('contacts.onlineServices.service')}
                  size="small"
                  fullWidth
                  value={row.service}
                  onChange={(e) => updateRow(index, { service: e.target.value })}
                />
              )}
              <TextField
                label={uriOnly ? t('contacts.onlineServices.impp') : t('contacts.onlineServices.uri')}
                size="small"
                fullWidth
                type="url"
                value={row.uri}
                onChange={(e) => updateRow(index, { uri: e.target.value })}
              />
              {!uriOnly && (
                <TextField
                  label={t('contacts.onlineServices.user')}
                  size="small"
                  fullWidth
                  value={row.user}
                  onChange={(e) => updateRow(index, { user: e.target.value })}
                />
              )}
              <Autocomplete
                multiple
                freeSolo
                size="small"
                options={CONTEXT_OPTIONS as readonly string[]}
                value={row.contexts}
                onChange={(_, newValue) => updateRow(index, { contexts: newValue as string[] })}
                getOptionLabel={(opt) => t(`contacts.contexts.${opt}`, opt)}
                renderInput={(params) => (
                  <TextField {...params} label={t('contacts.onlineServices.contexts')} size="small" />
                )}
              />
              <Stack direction="row" spacing={1} alignItems="center">
                <TextField
                  label={t('contacts.onlineServices.label')}
                  size="small"
                  fullWidth
                  value={row.label}
                  onChange={(e) => updateRow(index, { label: e.target.value })}
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
