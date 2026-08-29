import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import {
  Box,
  Button,
  FormControlLabel,
  IconButton,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { type FieldDefinition, type FieldValueEditorState, isMulti } from '../api/fieldDefinitions';
import { useDateFormat } from '../DateFormatProvider';
import { useRowKeys } from '../hooks/useRowKeys';

interface FieldValueEditorProps {
  definition: FieldDefinition;
  value: FieldValueEditorState;
  onChange: (next: FieldValueEditorState) => void;
}

// FieldValueEditor renders the per-<FieldDefinition.Type> input T7 calls for:
// a scalar editor (text/number/date/datetime/uri/email/phone/enum/boolean)
// or, when FieldConstraints.Multi is set, an add/remove list of that scalar
// editor -- reusing MultiValueField.tsx's add/remove-row pattern rather than
// inventing a second one. Value is the EDITOR state (see api/fieldDefinitions.ts's
// wireToEditorValue/editorToWireValue), not the raw wire JSON.
export default function FieldValueEditor({ definition, value, onChange }: FieldValueEditorProps) {
  const { t } = useTranslation();
  const { getBirthdayPlaceholder } = useDateFormat();
  const multi = isMulti(definition);
  const rowKeys = useRowKeys(multi && Array.isArray(value) ? value.length : 0);

  const updateRow = (index: number, next: string) => {
    const list = Array.isArray(value) ? [...value] : [];
    list[index] = next;
    onChange(list);
  };

  const removeRow = (index: number) => {
    const list = Array.isArray(value) ? [...value] : [];
    rowKeys.onRemove(index);
    onChange(list.filter((_, i) => i !== index));
  };

  const addRow = () => {
    const list = Array.isArray(value) ? [...value] : [];
    rowKeys.onAdd();
    onChange([...list, '']);
  };

  const renderScalar = (
    draft: string | boolean,
    onScalarChange: (next: string | boolean) => void,
  ) => {
    switch (definition.type) {
      case 'boolean':
        return (
          <FormControlLabel
            control={
              <Switch checked={draft === true} onChange={(e) => onScalarChange(e.target.checked)} />
            }
            label={t('customFields.booleanValue')}
          />
        );
      case 'enum': {
        const options = definition.constraints?.values || [];
        return (
          <TextField
            select
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            onChange={(e) => onScalarChange(e.target.value)}
          >
            <MenuItem value="">{t('customFields.selectValue')}</MenuItem>
            {options.map((opt) => (
              <MenuItem key={opt} value={opt}>
                {opt}
              </MenuItem>
            ))}
          </TextField>
        );
      }
      case 'number':
        return (
          <TextField
            type="number"
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
      case 'date':
        return (
          <TextField
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            placeholder={getBirthdayPlaceholder()}
            helperText={t('contacts.birthdayFormat')}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
      case 'datetime':
        return (
          <TextField
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            placeholder="2024-06-01T12:00:00Z"
            helperText={t('customFields.datetimeFormat')}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
      case 'email':
        return (
          <TextField
            type="email"
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
      case 'phone':
        return (
          <TextField
            type="tel"
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
      case 'uri':
        return (
          <TextField
            type="url"
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
      case 'text':
        return (
          <TextField
            multiline
            minRows={2}
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
      default:
        return (
          <TextField
            size="small"
            fullWidth
            label={t('customFields.value')}
            value={String(draft)}
            onChange={(e) => onScalarChange(e.target.value)}
          />
        );
    }
  };

  if (!multi) {
    return renderScalar(value as string | boolean, onChange);
  }

  return (
    <Box>
      <Stack spacing={1}>
        {(Array.isArray(value) ? value : []).map((row, index) => (
          <Stack
            key={rowKeys.keyAt(index)}
            direction="row"
            spacing={1}
            sx={{
              alignItems: 'center',
            }}
          >
            <Box sx={{ flexGrow: 1 }}>
              {renderScalar(row, (next) => updateRow(index, String(next)))}
            </Box>
            <IconButton
              size="small"
              color="error"
              onClick={() => removeRow(index)}
              aria-label={t('common.delete')}
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Stack>
        ))}
        <Box>
          <Button size="small" startIcon={<AddIcon />} onClick={addRow} variant="outlined">
            {t('common.add')}
          </Button>
        </Box>
      </Stack>
      <Typography
        variant="caption"
        sx={{
          color: 'text.secondary',
        }}
      >
        {t('customFields.multiHint')}
      </Typography>
    </Box>
  );
}
