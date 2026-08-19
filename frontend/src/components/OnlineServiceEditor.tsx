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
  SvgIcon,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { CardOnlineService, onlineServicesToRows, rowsToOnlineServices, OnlineServiceRow } from '../api/contacts';
import { LinkFieldType } from '../api/linkFieldTypes';
import { useRowKeys } from '../hooks/useRowKeys';
import { CONTEXT_OPTIONS } from '../contactFields';
import { resolveLinkFieldTypeIcon } from '../linkFieldTypeIcons';

interface OnlineServiceEditorProps {
  label: string;
  value: CardOnlineService[];
  onChange: (next: CardOnlineService[]) => void;
  /** When true, the service-name field is editable (social/other services). */
  showService?: boolean;
  /** When true, the URI field is the only value input (IMPP-style). */
  uriOnly?: boolean;
  /** The user's LinkFieldType registry (T44) — offered as freeSolo Autocomplete
   * suggestions for the service field so a registry entry is discoverable from
   * the editor itself. Optional: without it the field degrades to a plain
   * free-text input, exactly as it behaved before this ticket. */
  linkFieldTypes?: LinkFieldType[];
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
  linkFieldTypes,
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
      <Typography variant="subtitle2" component="p" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={1}>
        {rows.map((row, index) => (
          <Paper key={rowKeys.keyAt(index)} variant="outlined" sx={{ p: 1.5 }}>
            <Stack spacing={1}>
              {showService && (
                <Autocomplete
                  freeSolo
                  size="small"
                  options={linkFieldTypes ?? []}
                  value={row.service}
                  onChange={(_, newValue) => {
                    // A selected option arrives as a LinkFieldType object.
                    const service = typeof newValue === 'string' ? newValue : (newValue?.name ?? '');
                    updateRow(index, { service });
                  }}
                  onInputChange={(_, newInput, reason) => {
                    // Persist free text keystroke-by-keystroke (like the old
                    // TextField) so an unregistered/one-off service name
                    // survives even if the row is saved without committing
                    // the dropdown -- same freeSolo pattern MultiValueField
                    // uses for its type field.
                    if (reason === 'input') updateRow(index, { service: newInput });
                  }}
                  getOptionLabel={(opt) => (typeof opt === 'string' ? opt : opt.name)}
                  renderOption={({ key, ...optionProps }, opt) => (
                    <Box component="li" key={key} {...optionProps} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <SvgIcon fontSize="small">
                        <path d={resolveLinkFieldTypeIcon(opt.icon)} />
                      </SvgIcon>
                      {opt.name}
                    </Box>
                  )}
                  renderInput={(params) => (
                    <TextField {...params} label={t('contacts.onlineServices.service')} size="small" />
                  )}
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
