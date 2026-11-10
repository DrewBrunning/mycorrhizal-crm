import {
  Alert,
  Box,
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  TextField,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  createEntity,
  ENTITY_SENSITIVITIES,
  type Entity,
  type EntityKind,
  type EntitySensitivity,
  updateEntity,
} from '../api/entities';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import {
  coreToForm,
  type EntityForm,
  formToCore,
  type OrganisationForm,
  type PetForm,
  type PlaceForm,
} from '../utils/entityCore';
import { ENTITY_NEW_KEYS, ENTITY_SENSITIVITY_KEYS } from '../utils/entityLabels';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface EntityFormDialogProps {
  open: boolean;
  /** Required when creating; an edit uses the entity's own (immutable) kind. */
  kind?: EntityKind;
  entity?: Entity | null;
  onClose: () => void;
  onSaved: (entity: Entity) => void;
}

// Create / edit form for one entity (issue #2301, ADR 0043 decision 1). The
// kind is fixed once the entity exists, so an edit never offers it.
export default function EntityFormDialog({
  open,
  kind: createKind,
  entity,
  onClose,
  onSaved,
}: EntityFormDialogProps) {
  const { t } = useTranslation();
  const kind: EntityKind = entity?.kind ?? createKind ?? 'organisation';
  const [name, setName] = useState('');
  const [sensitivity, setSensitivity] = useState<EntitySensitivity>('normal');
  const [form, setForm] = useState<EntityForm>(() => coreToForm(kind, {}));
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const { markDirty, clearDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (!open) return;
    setName(entity?.name ?? '');
    setSensitivity(entity?.sensitivity ?? 'normal');
    setForm(coreToForm(kind, entity?.core ?? {}));
    setError('');
  }, [open, entity, kind]);

  const set = <T extends EntityForm>(patch: Partial<T>) => {
    markDirty();
    setError('');
    setForm((f) => ({ ...f, ...patch }) as EntityForm);
  };

  const handleSave = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      setError(t('entities.form.nameRequired'));
      return;
    }
    let core: Record<string, unknown>;
    try {
      core = formToCore(kind, trimmed, form, entity?.core);
    } catch (err) {
      const field = err instanceof Error ? err.message : 'birth';
      setError(
        t('entities.form.invalidDate', {
          field: field === 'death' ? t('entities.fields.death') : t('entities.fields.birth'),
        }),
      );
      return;
    }
    setSaving(true);
    try {
      const input = { kind, name: trimmed, sensitivity, core };
      const saved = entity
        ? await updateEntity(entity.id, input, entity.revision)
        : await createEntity(input);
      clearDirty();
      onSaved(saved);
    } catch (err) {
      setError(getErrorMessage(err) || t('entities.form.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const title = entity
    ? t('entities.form.editTitle', { name: entity.name })
    : t(ENTITY_NEW_KEYS[kind]);

  return (
    <AppDialog
      open={open}
      onClose={requestClose}
      maxWidth="sm"
      fullWidth
      aria-labelledby="entity-form-title"
    >
      <DialogTitle id="entity-form-title">{title}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label={t('entities.fields.name')}
            value={name}
            onChange={(e) => {
              markDirty();
              setError('');
              setName(e.target.value);
            }}
            slotProps={{ htmlInput: { maxLength: 200 } }}
            required
            fullWidth
          />
          {kind === 'organisation' && (
            <OrganisationFields form={form as OrganisationForm} set={set} />
          )}
          {kind === 'place' && <PlaceFields form={form as PlaceForm} set={set} />}
          {kind === 'pet' && <PetFields form={form as PetForm} set={set} />}
          <TextField
            select
            label={t('entities.sensitivityLabel')}
            value={sensitivity}
            onChange={(e) => {
              markDirty();
              setSensitivity(e.target.value as EntitySensitivity);
            }}
            fullWidth
          >
            {ENTITY_SENSITIVITIES.map((s) => (
              <MenuItem key={s} value={s}>
                {t(ENTITY_SENSITIVITY_KEYS[s])}
              </MenuItem>
            ))}
          </TextField>
          {error && <Alert severity="error">{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={requestClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button onClick={() => void handleSave()} variant="contained" disabled={saving}>
          {t('common.save')}
        </Button>
      </DialogActions>
      {guardElement}
    </AppDialog>
  );
}

type SetFn = <T extends EntityForm>(patch: Partial<T>) => void;

function NotesField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  return (
    <TextField
      label={t('entities.fields.notes')}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      slotProps={{ htmlInput: { maxLength: 5000 } }}
      multiline
      minRows={2}
      fullWidth
    />
  );
}

function OrganisationFields({ form, set }: { form: OrganisationForm; set: SetFn }) {
  const { t } = useTranslation();
  return (
    <>
      <TextField
        label={t('entities.fields.url')}
        value={form.url}
        onChange={(e) => set<OrganisationForm>({ url: e.target.value })}
        slotProps={{ htmlInput: { maxLength: 2000 } }}
        fullWidth
      />
      <TextField
        label={t('entities.fields.sortAs')}
        value={form.sortAs}
        onChange={(e) => set<OrganisationForm>({ sortAs: e.target.value })}
        fullWidth
      />
      <TextField
        label={t('entities.fields.units')}
        value={form.unitsText}
        onChange={(e) => set<OrganisationForm>({ unitsText: e.target.value })}
        multiline
        minRows={2}
        fullWidth
      />
      <NotesField value={form.notes} onChange={(v) => set<OrganisationForm>({ notes: v })} />
    </>
  );
}

function PlaceFields({ form, set }: { form: PlaceForm; set: SetFn }) {
  const { t } = useTranslation();
  const labels = {
    street: t('entities.fields.street'),
    city: t('entities.fields.city'),
    region: t('entities.fields.region'),
    postal: t('entities.fields.postal'),
    country: t('entities.fields.country'),
  };
  const field = (key: keyof typeof labels) => (
    <TextField
      label={labels[key]}
      value={form[key]}
      onChange={(e) => set<PlaceForm>({ [key]: e.target.value })}
      fullWidth
    />
  );
  return (
    <>
      {field('street')}
      {field('city')}
      {field('region')}
      {field('postal')}
      {field('country')}
      <NotesField value={form.notes} onChange={(v) => set<PlaceForm>({ notes: v })} />
    </>
  );
}

function PetFields({ form, set }: { form: PetForm; set: SetFn }) {
  const { t } = useTranslation();
  return (
    <>
      <TextField
        label={t('entities.fields.species')}
        value={form.species}
        onChange={(e) => set<PetForm>({ species: e.target.value })}
        slotProps={{ htmlInput: { maxLength: 100 } }}
        fullWidth
      />
      <TextField
        label={t('entities.fields.breed')}
        value={form.breed}
        onChange={(e) => set<PetForm>({ breed: e.target.value })}
        slotProps={{ htmlInput: { maxLength: 100 } }}
        fullWidth
      />
      <TextField
        label={t('entities.fields.birth')}
        helperText={t('entities.fields.dateHint')}
        value={form.birth}
        onChange={(e) => set<PetForm>({ birth: e.target.value })}
        fullWidth
      />
      <TextField
        label={t('entities.fields.death')}
        helperText={t('entities.fields.dateHint')}
        value={form.death}
        onChange={(e) => set<PetForm>({ death: e.target.value })}
        fullWidth
      />
      <NotesField value={form.notes} onChange={(v) => set<PetForm>({ notes: v })} />
    </>
  );
}
