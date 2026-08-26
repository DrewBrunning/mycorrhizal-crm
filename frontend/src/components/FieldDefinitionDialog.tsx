import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import {
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  FIELD_TYPES,
  type FieldDefinition,
  type FieldDefinitionInput,
  type FieldSensitivity,
  type FieldType,
} from '../api/fieldDefinitions';
import { useRowKeys } from '../hooks/useRowKeys';
import AppDialog from './AppDialog';

interface FieldDefinitionDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (input: FieldDefinitionInput) => Promise<void>;
  definition?: FieldDefinition | null; // null/undefined = create mode
}

interface FormState {
  label: string;
  key: string;
  type: FieldType;
  multi: boolean;
  min: string;
  max: string;
  maxLength: string;
  pattern: string;
  enumValues: string[];
  projectionMode: 'internal' | 'vcard';
  vcardName: string;
  sensitivity: FieldSensitivity;
}

const emptyForm: FormState = {
  label: '',
  key: '',
  type: 'string',
  multi: false,
  min: '',
  max: '',
  maxLength: '',
  pattern: '',
  enumValues: [],
  projectionMode: 'internal',
  vcardName: '',
  sensitivity: 'normal',
};

// FieldDefinitionDialog creates and edits a FieldDefinition (T7's replacement
// for v1's name-only editor). Key is read-only in edit mode: it is the stable
// machine name and the backend rejects changes to it on update.
export default function FieldDefinitionDialog({
  open,
  onClose,
  onSave,
  definition,
}: FieldDefinitionDialogProps) {
  const { t } = useTranslation();
  const isEditing = !!definition;
  const [form, setForm] = useState<FormState>(emptyForm);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const enumRowKeys = useRowKeys(form.enumValues.length);

  useEffect(() => {
    if (open) {
      if (definition) {
        const constraints = definition.constraints || {};
        const projection = definition.projection || 'internal-only';
        setForm({
          label: definition.label,
          key: definition.key,
          type: definition.type,
          multi: !!constraints.multi,
          min: constraints.min != null ? String(constraints.min) : '',
          max: constraints.max != null ? String(constraints.max) : '',
          maxLength: constraints.maxLength != null ? String(constraints.maxLength) : '',
          pattern: constraints.pattern || '',
          enumValues: constraints.values ? [...constraints.values] : [],
          projectionMode: projection.startsWith('vcard:') ? 'vcard' : 'internal',
          vcardName: projection.replace(/^vcard:X-/, ''),
          sensitivity: definition.sensitivity,
        });
      } else {
        setForm(emptyForm);
      }
      setError('');
    }
  }, [open, definition]);

  const set = (patch: Partial<FormState>) => setForm((prev) => ({ ...prev, ...patch }));

  const updateEnumRow = (index: number, next: string) => {
    set({ enumValues: form.enumValues.map((v, i) => (i === index ? next : v)) });
  };
  const removeEnumRow = (index: number) => {
    enumRowKeys.onRemove(index);
    set({ enumValues: form.enumValues.filter((_, i) => i !== index) });
  };
  const addEnumRow = () => {
    enumRowKeys.onAdd();
    set({ enumValues: [...form.enumValues, ''] });
  };

  const handleSave = async () => {
    if (!form.label.trim()) {
      setError(t('customFields.labelRequired'));
      return;
    }
    if (!isEditing && !form.key.trim()) {
      setError(t('customFields.keyRequired'));
      return;
    }
    if (form.type === 'enum' && form.enumValues.filter((v) => v.trim()).length === 0) {
      setError(t('customFields.enumValuesRequired'));
      return;
    }
    if (form.projectionMode === 'vcard' && !form.vcardName.trim()) {
      setError(t('customFields.vcardNameRequired'));
      return;
    }

    const constraints: Record<string, unknown> = {};
    if (form.multi) constraints.multi = true;
    if (form.type === 'number') {
      if (form.min !== '') constraints.min = Number(form.min);
      if (form.max !== '') constraints.max = Number(form.max);
    }
    if (form.type === 'string' || form.type === 'text') {
      if (form.maxLength !== '') constraints.maxLength = Number(form.maxLength);
      if (form.pattern.trim()) constraints.pattern = form.pattern.trim();
    }
    if (form.type === 'enum') {
      const values = form.enumValues.map((v) => v.trim()).filter(Boolean);
      if (values.length > 0) constraints.values = values;
    }

    const input: FieldDefinitionInput = {
      label: form.label.trim(),
      key: form.key.trim(),
      type: form.type,
      constraints:
        Object.keys(constraints).length > 0
          ? (constraints as FieldDefinitionInput['constraints'])
          : undefined,
      projection:
        form.projectionMode === 'vcard' ? `vcard:X-${form.vcardName.trim()}` : 'internal-only',
      sensitivity: form.sensitivity,
    };

    setSaving(true);
    setError('');
    try {
      await onSave(input);
      handleClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : t('customFields.saveError'));
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    setForm(emptyForm);
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {isEditing ? t('customFields.editTitle') : t('customFields.addTitle')}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {error && (
            <Typography color="error" sx={{ fontSize: '0.875rem' }}>
              {error}
            </Typography>
          )}

          <TextField
            label={t('customFields.label')}
            value={form.label}
            onChange={(e) => set({ label: e.target.value })}
            fullWidth
            required
            helperText={t('customFields.labelHint')}
          />
          <TextField
            label={t('customFields.key')}
            value={form.key}
            onChange={(e) => set({ key: e.target.value })}
            fullWidth
            required
            disabled={isEditing}
            helperText={t('customFields.keyHint')}
          />

          <TextField
            select
            label={t('customFields.type')}
            value={form.type}
            onChange={(e) => set({ type: e.target.value as FieldType })}
            fullWidth
          >
            {FIELD_TYPES.map((type) => (
              <MenuItem key={type} value={type}>
                {t(`customFields.types.${type}`)}
              </MenuItem>
            ))}
          </TextField>

          <FormControlLabel
            control={
              <Switch checked={form.multi} onChange={(e) => set({ multi: e.target.checked })} />
            }
            label={t('customFields.multi')}
          />

          {form.type === 'string' || form.type === 'text' ? (
            <Stack direction="row" spacing={2}>
              <TextField
                label={t('customFields.maxLength')}
                type="number"
                value={form.maxLength}
                onChange={(e) => set({ maxLength: e.target.value })}
                sx={{ width: 180 }}
              />
              <TextField
                label={t('customFields.pattern')}
                value={form.pattern}
                onChange={(e) => set({ pattern: e.target.value })}
                fullWidth
                helperText={t('customFields.patternHint')}
              />
            </Stack>
          ) : form.type === 'number' ? (
            <Stack direction="row" spacing={2}>
              <TextField
                label={t('customFields.min')}
                type="number"
                value={form.min}
                onChange={(e) => set({ min: e.target.value })}
              />
              <TextField
                label={t('customFields.max')}
                type="number"
                value={form.max}
                onChange={(e) => set({ max: e.target.value })}
              />
            </Stack>
          ) : form.type === 'enum' ? (
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                {t('customFields.enumValues')}
              </Typography>
              <Stack spacing={1}>
                {form.enumValues.map((value, index) => (
                  <Stack
                    key={enumRowKeys.keyAt(index)}
                    direction="row"
                    spacing={1}
                    alignItems="center"
                  >
                    <TextField
                      size="small"
                      fullWidth
                      value={value}
                      onChange={(e) => updateEnumRow(index, e.target.value)}
                    />
                    <IconButton
                      size="small"
                      color="error"
                      onClick={() => removeEnumRow(index)}
                      aria-label={t('common.delete')}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </Stack>
                ))}
                <Box>
                  <Button
                    size="small"
                    startIcon={<AddIcon />}
                    onClick={addEnumRow}
                    variant="outlined"
                  >
                    {t('common.add')}
                  </Button>
                </Box>
              </Stack>
            </Box>
          ) : null}

          <TextField
            select
            label={t('customFields.projection')}
            value={form.projectionMode}
            onChange={(e) => set({ projectionMode: e.target.value as FormState['projectionMode'] })}
            fullWidth
          >
            <MenuItem value="internal">{t('customFields.projectionInternal')}</MenuItem>
            <MenuItem value="vcard">{t('customFields.projectionVcard')}</MenuItem>
          </TextField>
          {form.projectionMode === 'vcard' && (
            <TextField
              label={t('customFields.vcardName')}
              value={form.vcardName}
              onChange={(e) => set({ vcardName: e.target.value })}
              fullWidth
              required
              helperText={t('customFields.vcardNameHint')}
            />
          )}

          <TextField
            select
            label={t('customFields.sensitivity')}
            value={form.sensitivity}
            onChange={(e) => set({ sensitivity: e.target.value as FieldSensitivity })}
            fullWidth
          >
            <MenuItem value="normal">{t('customFields.sensitivityNormal')}</MenuItem>
            <MenuItem value="private">{t('customFields.sensitivityPrivate')}</MenuItem>
            <MenuItem value="secret">{t('customFields.sensitivitySecret')}</MenuItem>
          </TextField>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
