import {
  Alert,
  Button,
  Checkbox,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControlLabel,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  adoptContactEntityLink,
  deleteEntity,
  type Entity,
  type EntityContactLink,
  type EntityDeleteResponse,
} from '../api/entities';
import { isLinkCurrent, localToday } from '../utils/entityPeriod';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';

interface EntityDeleteDialogProps {
  open: boolean;
  entity: Entity;
  /** The entity's live links; used to find the contacts that would lose a projected value. */
  contacts: EntityContactLink[];
  onClose: () => void;
  onDeleted: (result: EntityDeleteResponse) => void;
}

// Delete confirmation (ADR 0043 decision 6): says how many contacts currently
// receive a projected ORG/ADR from this entity, and offers "adopt all" -- copy
// the value into each of those contacts as their own before the delete. Adopt
// is never automatic; it is a checkbox the user ticks.
export default function EntityDeleteDialog({
  open,
  entity,
  contacts,
  onClose,
  onDeleted,
}: EntityDeleteDialogProps) {
  const { t } = useTranslation();
  const [adopt, setAdopt] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const projection = entity.projection_count ?? 0;
  const links = entity.link_count ?? 0;
  const projects = entity.kind === 'organisation' || entity.kind === 'place';
  // The contacts that currently receive the projection: primary links whose
  // period includes today. (The server's projection_count is authoritative for
  // the sentence; this only picks whom to copy the value into.)
  const today = localToday();
  const adoptable = projects
    ? contacts.filter((c) => c.is_primary && isLinkCurrent(c.period, today))
    : [];

  const projectsKey =
    entity.kind === 'place'
      ? 'entities.delete.projectsPlace'
      : 'entities.delete.projectsOrganisation';
  const adoptKey =
    entity.kind === 'place' ? 'entities.delete.adoptPlace' : 'entities.delete.adoptOrganisation';

  const handleDelete = async () => {
    setBusy(true);
    setError('');
    try {
      if (adopt && adoptable.length > 0) {
        let failed = 0;
        for (const c of adoptable) {
          try {
            await adoptContactEntityLink(c.contact_id, c.link_id);
          } catch {
            failed++;
          }
        }
        if (failed > 0) {
          setError(t('entities.delete.adoptFailed', { count: failed }));
          return;
        }
      }
      onDeleted(await deleteEntity(entity.id, entity.revision));
    } catch (err) {
      setError(getErrorMessage(err) || t('entities.delete.failed'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AppDialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      aria-labelledby="entity-delete-title"
    >
      <DialogTitle id="entity-delete-title">
        {t('entities.delete.title', { name: entity.name })}
      </DialogTitle>
      <DialogContent>
        {projects && projection > 0 ? (
          <DialogContentText sx={{ mb: 1 }}>
            {t(projectsKey, { name: entity.name, count: projection })}
          </DialogContentText>
        ) : (
          <DialogContentText sx={{ mb: 1 }}>
            {t('entities.delete.plain', { name: entity.name })}
          </DialogContentText>
        )}
        {links > 0 && (
          <DialogContentText sx={{ mb: 1 }}>
            {t('entities.delete.links', { count: links })}
          </DialogContentText>
        )}
        {adoptable.length > 0 && (
          <FormControlLabel
            control={<Checkbox checked={adopt} onChange={(e) => setAdopt(e.target.checked)} />}
            label={t(adoptKey)}
          />
        )}
        {error && (
          <Alert severity="error" sx={{ mt: 1 }}>
            {error}
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={busy}>
          {t('common.cancel')}
        </Button>
        <Button
          color="error"
          variant="contained"
          onClick={() => void handleDelete()}
          disabled={busy}
        >
          {t('entities.delete.confirm')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
