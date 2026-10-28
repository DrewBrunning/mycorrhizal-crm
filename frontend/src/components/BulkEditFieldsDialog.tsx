import {
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  BULK_EDITABLE_FIELDS,
  type BulkEditableField,
  type BulkOperationRequest,
} from '../api/bulkOperations';
import type { FieldDefinition } from '../api/fieldDefinitions';

// Custom-field types the dialog can edit as plain text. Numbers, booleans,
// dates, enums and multi-valued definitions need typed inputs; they stay
// editable per contact.
const TEXT_CUSTOM_TYPES = new Set(['string', 'text', 'uri', 'email', 'phone']);

export type BulkFieldsPayload = Pick<
  BulkOperationRequest,
  'set' | 'clear' | 'custom_set' | 'custom_clear'
>;

interface BulkEditFieldsDialogProps {
  open: boolean;
  selectedCount: number;
  busy: boolean;
  fieldDefinitions: FieldDefinition[];
  onClose: () => void;
  onSubmit: (payload: BulkFieldsPayload) => void;
}

interface FieldState {
  value: string;
  clear: boolean;
}

// "Edit fields" dialog for the bulk-action bar (issue #1996). A blank field
// that is not marked Clear is left untouched; a filled field is written; a
// Clear-marked field is emptied. The server enforces the same allowlist.
// The parent mounts it only while open, so state starts fresh every time.
export default function BulkEditFieldsDialog({
  open,
  selectedCount,
  busy,
  fieldDefinitions,
  onClose,
  onSubmit,
}: BulkEditFieldsDialogProps) {
  const { t } = useTranslation();
  const [state, setState] = useState<Record<string, FieldState>>({});

  // Literal keys, so the i18n key-usage guard can see every label is used.
  const fieldLabels: Record<BulkEditableField, string> = {
    organization: t('bulk.field_organization'),
    department: t('bulk.field_department'),
    job_title: t('bulk.field_job_title'),
    role: t('bulk.field_role'),
    gender: t('bulk.field_gender'),
    how_we_met: t('bulk.field_how_we_met'),
    work_information: t('bulk.field_work_information'),
    contact_information: t('bulk.field_contact_information'),
  };
  const customDefs = fieldDefinitions.filter(
    (d) => TEXT_CUSTOM_TYPES.has(d.type) && !d.constraints?.multi,
  );
  const get = (key: string): FieldState => state[key] ?? { value: '', clear: false };
  const patch = (key: string, next: Partial<FieldState>) =>
    setState((prev) => ({
      ...prev,
      [key]: { ...get(key), ...prev[key], ...next },
    }));

  const buildPayload = (): BulkFieldsPayload => {
    const set: NonNullable<BulkFieldsPayload['set']> = {};
    const clear: NonNullable<BulkFieldsPayload['clear']> = [];
    for (const key of BULK_EDITABLE_FIELDS) {
      const s = get(key);
      if (s.clear) clear.push(key);
      else if (s.value.trim() !== '') set[key] = s.value;
    }
    const customSet: NonNullable<BulkFieldsPayload['custom_set']> = [];
    const customClear: string[] = [];
    for (const def of customDefs) {
      const s = get(def.id);
      if (s.clear) customClear.push(def.id);
      else if (s.value.trim() !== '')
        customSet.push({ field_definition_id: def.id, value: s.value });
    }
    return {
      ...(Object.keys(set).length > 0 ? { set } : {}),
      ...(clear.length > 0 ? { clear } : {}),
      ...(customSet.length > 0 ? { custom_set: customSet } : {}),
      ...(customClear.length > 0 ? { custom_clear: customClear } : {}),
    };
  };

  const payload = buildPayload();
  const hasChanges = Object.keys(payload).length > 0;

  const row = (key: string, label: string, maxLength?: number) => {
    const s = get(key);
    return (
      <Stack key={key} direction="row" spacing={1} sx={{ alignItems: 'center' }}>
        <TextField
          size="small"
          fullWidth
          label={label}
          value={s.clear ? '' : s.value}
          disabled={busy || s.clear}
          onChange={(e) => patch(key, { value: e.target.value })}
          slotProps={{ htmlInput: { maxLength } }}
        />
        <FormControlLabel
          sx={{ flexShrink: 0, mr: 0 }}
          control={
            <Checkbox
              size="small"
              checked={s.clear}
              disabled={busy}
              onChange={(e) => patch(key, { clear: e.target.checked })}
              slotProps={{
                input: {
                  'aria-label': `${t('bulk.editFieldsClear')}: ${label}`,
                },
              }}
            />
          }
          label={t('bulk.editFieldsClear')}
        />
      </Stack>
    );
  };

  return (
    <Dialog open={open} onClose={busy ? undefined : onClose} fullWidth maxWidth="sm">
      <DialogTitle>{t('bulk.editFieldsTitle', { count: selectedCount })}</DialogTitle>
      <DialogContent>
        <Stack spacing={1.5} sx={{ pt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            {t('bulk.editFieldsHint')}
          </Typography>
          {BULK_EDITABLE_FIELDS.map((key) =>
            row(
              key,
              fieldLabels[key],
              key === 'gender'
                ? 100
                : key === 'how_we_met' ||
                    key === 'work_information' ||
                    key === 'contact_information'
                  ? 1000
                  : 200,
            ),
          )}
          {customDefs.length > 0 && (
            <>
              <Typography variant="subtitle2">{t('bulk.editFieldsCustomHeading')}</Typography>
              {customDefs.map((def) => row(def.id, def.label))}
            </>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={busy}>
          {t('common.cancel')}
        </Button>
        <Button
          variant="contained"
          disabled={busy || !hasChanges}
          onClick={() => onSubmit(payload)}
        >
          {t('bulk.editFieldsApply')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
