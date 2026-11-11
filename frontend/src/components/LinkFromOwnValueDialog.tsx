import {
  Alert,
  Button,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControl,
  FormControlLabel,
  MenuItem,
  Radio,
  RadioGroup,
  TextField,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ContactEntityLinkFromExistingInput,
  ENTITY_LINK_ROLES,
  type Entity,
  type EntityKind,
  listEntities,
} from '../api/entities';
import { roleKey } from '../utils/entityLabels';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';

export interface OwnValue {
  /** Which neutral-card property it is: ORG or ADR. */
  kind: 'organization' | 'address';
  /** The card element id, when it has one; otherwise it is addressed by position. */
  id?: string;
  /** Position among the contact's own (non-projected) entries of this kind. */
  index: number;
  label: string;
}

interface LinkFromOwnValueDialogProps {
  open: boolean;
  values: OwnValue[];
  onClose: () => void;
  onLink: (input: ContactEntityLinkFromExistingInput) => Promise<void>;
}

const entityKindOf = (v: OwnValue): EntityKind =>
  v.kind === 'organization' ? 'organisation' : 'place';

// "Link from existing" (ADR 0043 decision 3): connect one of the contact's own
// ORG/ADR values to a shared entity -- an existing one the user picks, or a new
// one created from the value. The contact's own value is never removed.
export default function LinkFromOwnValueDialog({
  open,
  values,
  onClose,
  onLink,
}: LinkFromOwnValueDialogProps) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState(0);
  const [role, setRole] = useState('employee');
  const [matches, setMatches] = useState<Entity[]>([]);
  const [target, setTarget] = useState('new');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  const value: OwnValue | undefined = values[selected];
  const kind: EntityKind | null = value ? entityKindOf(value) : null;

  useEffect(() => {
    if (open) {
      setSelected(0);
      setError('');
    }
  }, [open]);

  useEffect(() => {
    if (!open || !kind) return;
    setRole(ENTITY_LINK_ROLES[kind][0]);
    setTarget('new');
  }, [open, kind, selected]);

  // Only organisations can be matched by name: a place's name is the user's own
  // label, not the postal text the contact's address carries.
  const matchName = value?.kind === 'organization' ? value.label : '';
  useEffect(() => {
    if (!open || !matchName || !kind) {
      setMatches([]);
      return;
    }
    let cancelled = false;
    listEntities({ kind, q: matchName, limit: 5 })
      .then((page) => {
        if (!cancelled) setMatches(page.entities);
      })
      .catch(() => {
        if (!cancelled) setMatches([]);
      });
    return () => {
      cancelled = true;
    };
  }, [open, matchName, kind]);

  const roles = useMemo(() => (kind ? ENTITY_LINK_ROLES[kind] : []), [kind]);

  const handleLink = async () => {
    if (!value || !kind) return;
    setSaving(true);
    setError('');
    try {
      await onLink({
        ...(target !== 'new' ? { entity_id: target } : {}),
        role,
        source: {
          kind: value.kind,
          ...(value.id ? { id: value.id } : { index: value.index }),
        },
      });
      onClose();
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const newLabel =
    kind === 'place'
      ? t('contactDetail.entityLinks.fromOwn.createNewPlace')
      : t('contactDetail.entityLinks.fromOwn.createNewOrganisation');

  return (
    <AppDialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      aria-labelledby="entity-from-own-title"
    >
      <DialogTitle id="entity-from-own-title">
        {t('contactDetail.entityLinks.fromOwn.title')}
      </DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>
          {t('contactDetail.entityLinks.fromOwn.intro')}
        </DialogContentText>
        <TextField
          select
          label={t('contactDetail.entityLinks.fromOwn.value')}
          value={selected}
          onChange={(e) => setSelected(Number(e.target.value))}
          fullWidth
          sx={{ mb: 2 }}
        >
          {values.map((v, i) => (
            <MenuItem key={`${v.kind}:${v.id ?? v.index}`} value={i}>
              {v.label}
            </MenuItem>
          ))}
        </TextField>
        {kind && (
          <TextField
            select
            label={t('contactDetail.entityLinks.role')}
            value={role}
            onChange={(e) => setRole(e.target.value)}
            fullWidth
            sx={{ mb: 2 }}
          >
            {roles.map((r) => (
              <MenuItem key={r} value={r}>
                {t(roleKey(kind, r) ?? 'entities.roles.organisation.other')}
              </MenuItem>
            ))}
          </TextField>
        )}
        <FormControl>
          <RadioGroup value={target} onChange={(e) => setTarget(e.target.value)}>
            <FormControlLabel value="new" control={<Radio />} label={newLabel} />
            {matches.map((m) => (
              <FormControlLabel
                key={m.id}
                value={m.id}
                control={<Radio />}
                label={t('contactDetail.entityLinks.fromOwn.useExistingOrganisation', {
                  name: m.name,
                })}
              />
            ))}
          </RadioGroup>
        </FormControl>
        {error && (
          <Alert severity="error" sx={{ mt: 1 }}>
            {error}
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button variant="contained" onClick={() => void handleLink()} disabled={saving || !value}>
          {t('contactDetail.entityLinks.fromOwn.link')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
