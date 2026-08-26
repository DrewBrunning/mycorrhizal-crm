import ClearAllIcon from '@mui/icons-material/ClearAll';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Button, CircularProgress, IconButton, Stack, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  editorToWireValue,
  type FieldDefinition,
  fieldValueToDisplay,
  isEditorValueEmpty,
  wireToEditorValue,
} from '../api/fieldDefinitions';
import { useSnackbar } from '../context/SnackbarContext';
import { getErrorMessage } from '../utils/errorHandler';
import FieldValueEditor from './FieldValueEditor';

interface CustomFieldValueRowProps {
  definition: FieldDefinition;
  value: unknown; // raw wire value, undefined when the contact has none
  onSave: (definitionId: string, value: unknown) => Promise<void>;
}

// CustomFieldValueRow is one v2 custom field on a contact's detail page:
// a read-only display of the current value plus an inline editor rendered
// per FieldDefinition.Type (via FieldValueEditor). Saving passes the new raw
// wire value up to the caller, which is responsible for the full-replace
// semantics of PUT /contacts/:id/field-values; null signals "remove this
// definition's value".
export default function CustomFieldValueRow({
  definition,
  value,
  onSave,
}: CustomFieldValueRowProps) {
  const { t } = useTranslation();
  const { showError } = useSnackbar();
  const [editing, setEditing] = useState(false);
  const [editorValue, setEditorValue] = useState(() => wireToEditorValue(definition, value));
  const [saving, setSaving] = useState(false);

  const startEdit = () => {
    setEditorValue(wireToEditorValue(definition, value));
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
    setEditorValue(wireToEditorValue(definition, value));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const next = isEditorValueEmpty(definition, editorValue)
        ? null
        : editorToWireValue(definition, editorValue);
      await onSave(definition.id, next);
      setEditing(false);
    } catch (err) {
      showError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const display =
    value === undefined || value === null ? '' : fieldValueToDisplay(definition, value);

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <ClearAllIcon sx={{ mr: 1, color: 'text.secondary', fontSize: '1.2rem' }} />
        <Typography variant="body2" sx={{ fontWeight: 500, mr: 1 }}>
          {definition.label}
        </Typography>
        {!editing && (
          <IconButton size="small" onClick={startEdit} aria-label={t('common.edit')}>
            <EditIcon fontSize="small" />
          </IconButton>
        )}
      </Box>
      {editing ? (
        <Stack spacing={1} sx={{ mt: 1 }}>
          <FieldValueEditor definition={definition} value={editorValue} onChange={setEditorValue} />
          <Stack direction="row" spacing={1} justifyContent="flex-end">
            <Button size="small" onClick={cancelEdit} disabled={saving}>
              {t('common.cancel')}
            </Button>
            <Button size="small" variant="contained" onClick={handleSave} disabled={saving}>
              {saving ? <CircularProgress size={16} color="inherit" /> : t('common.save')}
            </Button>
          </Stack>
        </Stack>
      ) : (
        <Typography
          variant="body2"
          color={display ? 'text.primary' : 'text.disabled'}
          sx={{ pl: '2.2rem' }}
        >
          {display || '—'}
        </Typography>
      )}
    </Box>
  );
}
