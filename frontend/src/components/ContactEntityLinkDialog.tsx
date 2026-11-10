import {
  Alert,
  Box,
  Button,
  Checkbox,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  TextField,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ContactEntityLink,
  type ContactEntityLinkInput,
  createEntity,
  ENTITY_KINDS,
  ENTITY_LINK_ROLES,
  ENTITY_SENSITIVITIES,
  type EntityKind,
  type EntitySensitivity,
} from '../api/entities';
import { useDirtyDialog } from '../hooks/useDirtyDialog';
import { coreToForm, formToCore } from '../utils/entityCore';
import { ENTITY_KIND_KEYS, ENTITY_SENSITIVITY_KEYS, roleKey } from '../utils/entityLabels';
import { periodFromYears, yearsFromPeriod } from '../utils/entityPeriod';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';
import EntityPicker, { type EntityChoice } from './EntityPicker';

interface ContactEntityLinkDialogProps {
  open: boolean;
  /** The link being edited; null to add a new one. */
  link: ContactEntityLink | null;
  onClose: () => void;
  onSave: (input: ContactEntityLinkInput, editing: ContactEntityLink | null) => Promise<void>;
}

// Add / edit one contact-entity link (ADR 0043 decision 2): which entity, the
// closed role plus an optional free-text qualifier, whole-year dates, the
// primary flag (the projection switch) and the link's own sensitivity.
export default function ContactEntityLinkDialog({
  open,
  link,
  onClose,
  onSave,
}: ContactEntityLinkDialogProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<EntityKind>('organisation');
  const [choice, setChoice] = useState<EntityChoice | null>(null);
  const [role, setRole] = useState('employee');
  const [roleLabel, setRoleLabel] = useState('');
  const [primary, setPrimary] = useState(false);
  const [startYear, setStartYear] = useState('');
  const [endYear, setEndYear] = useState('');
  const [sensitivity, setSensitivity] = useState<EntitySensitivity>('normal');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const { markDirty, clearDirty, requestClose, guardElement } = useDirtyDialog(open, onClose);

  useEffect(() => {
    if (!open) return;
    const k = link?.kind ?? 'organisation';
    setKind(k);
    setChoice(null);
    setRole(link?.role ?? ENTITY_LINK_ROLES[k][0]);
    setRoleLabel(link?.role_label ?? '');
    setPrimary(link?.is_primary ?? false);
    const years = yearsFromPeriod(link?.period);
    setStartYear(years.startYear);
    setEndYear(years.endYear);
    setSensitivity(link?.sensitivity ?? 'normal');
    setError('');
  }, [open, link]);

  const edit =
    <T,>(setter: (v: T) => void) =>
    (v: T) => {
      markDirty();
      setError('');
      setter(v);
    };

  const changeKind = (next: EntityKind) => {
    markDirty();
    setKind(next);
    setChoice(null);
    setRole(ENTITY_LINK_ROLES[next][0]);
    if (next === 'pet') setPrimary(false);
  };

  const handleSave = async () => {
    if (!link && !choice) {
      setError(t('contactDetail.entityLinks.entityRequired'));
      return;
    }
    const period = periodFromYears(startYear, endYear, link?.period);
    if (period === 'invalid') {
      setError(t('contactDetail.entityLinks.yearInvalid'));
      return;
    }
    setSaving(true);
    try {
      let entityId = link?.entity_id;
      if (!entityId && choice) {
        if (choice.create) {
          const core = formToCore(kind, choice.name, coreToForm(kind, {}));
          entityId = (await createEntity({ kind, name: choice.name, core, sensitivity: 'normal' }))
            .id;
        } else {
          entityId = choice.entity.id;
        }
      }
      await onSave(
        {
          entity_id: entityId,
          role,
          role_label: roleLabel.trim(),
          is_primary: kind === 'pet' ? false : primary,
          ...(period ? { period } : {}),
          sensitivity,
        },
        link,
      );
      clearDirty();
      onClose();
    } catch (err) {
      setError(getErrorMessage(err) || t('contactDetail.entityLinks.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const title = link
    ? t('contactDetail.entityLinks.editTitle', { name: link.entity_name })
    : t('contactDetail.entityLinks.addTitle');

  return (
    <AppDialog
      open={open}
      onClose={requestClose}
      maxWidth="sm"
      fullWidth
      aria-labelledby="entity-link-title"
    >
      <DialogTitle id="entity-link-title">{title}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {!link && (
            <>
              <TextField
                select
                label={t('contactDetail.entityLinks.kind')}
                value={kind}
                onChange={(e) => changeKind(e.target.value as EntityKind)}
                fullWidth
              >
                {ENTITY_KINDS.map((k) => (
                  <MenuItem key={k} value={k}>
                    {t(ENTITY_KIND_KEYS[k])}
                  </MenuItem>
                ))}
              </TextField>
              <EntityPicker
                kind={kind}
                value={choice}
                onChange={(c) => {
                  markDirty();
                  setError('');
                  setChoice(c);
                }}
              />
            </>
          )}
          <TextField
            select
            label={t('contactDetail.entityLinks.role')}
            value={role}
            onChange={(e) => edit(setRole)(e.target.value)}
            fullWidth
          >
            {ENTITY_LINK_ROLES[kind].map((r) => (
              <MenuItem key={r} value={r}>
                {t(roleKey(kind, r) ?? 'entities.roles.organisation.other')}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            label={t('contactDetail.entityLinks.roleLabel')}
            value={roleLabel}
            onChange={(e) => edit(setRoleLabel)(e.target.value)}
            slotProps={{ htmlInput: { maxLength: 100 } }}
            fullWidth
          />
          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              label={t('contactDetail.period.from')}
              value={startYear}
              onChange={(e) => edit(setStartYear)(e.target.value)}
              slotProps={{ htmlInput: { inputMode: 'numeric', maxLength: 4 } }}
              fullWidth
            />
            <TextField
              label={t('contactDetail.period.to')}
              value={endYear}
              onChange={(e) => edit(setEndYear)(e.target.value)}
              slotProps={{ htmlInput: { inputMode: 'numeric', maxLength: 4 } }}
              fullWidth
            />
          </Box>
          {kind !== 'pet' && (
            <FormControlLabel
              control={
                <Checkbox checked={primary} onChange={(e) => edit(setPrimary)(e.target.checked)} />
              }
              label={t('contactDetail.entityLinks.primaryHelp')}
            />
          )}
          <TextField
            select
            label={t('entities.sensitivityLabel')}
            value={sensitivity}
            onChange={(e) => edit(setSensitivity)(e.target.value as EntitySensitivity)}
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
        <Button variant="contained" onClick={() => void handleSave()} disabled={saving}>
          {t('common.save')}
        </Button>
      </DialogActions>
      {guardElement}
    </AppDialog>
  );
}
