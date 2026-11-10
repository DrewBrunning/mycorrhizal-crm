import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import {
  Alert,
  AlertTitle,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Link as MuiLink,
  Stack,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate, useParams } from 'react-router';
import { ApiError } from './api/client';
import {
  type Entity,
  type EntityContactLink,
  getEntity,
  listEntityContacts,
  listEntityFieldValues,
  replaceEntityFieldValues,
  restoreEntity,
} from './api/entities';
import { type FieldDefinition, type FieldValue, getFieldDefinitions } from './api/fieldDefinitions';
import CustomFieldGroups from './components/CustomFieldGroups';
import EntityDeleteDialog from './components/EntityDeleteDialog';
import EntityFormDialog from './components/EntityFormDialog';
import EntityMergeDialog from './components/EntityMergeDialog';
import LoadError from './components/LoadError';
import PageHeader from './components/PageHeader';
import { useSnackbar } from './context/SnackbarContext';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { fieldValueInputsWith } from './utils/contactDetailPayloads';
import { coreToForm } from './utils/entityCore';
import { ENTITY_KIND_KEYS, ENTITY_SENSITIVITY_KEYS, roleKey } from './utils/entityLabels';
import { formatLinkPeriod } from './utils/entityPeriod';
import { getErrorMessage } from './utils/errorHandler';

interface MergedLoserState {
  mergedLoser?: { id: string; name: string };
}

export default function EntityDetailPage() {
  const { id = '' } = useParams();
  const { t } = useTranslation();
  const { showSuccess, showError, showWarning } = useSnackbar();
  const navigate = useNavigate();
  const location = useLocation();
  const mergedLoser = (location.state as MergedLoserState | null)?.mergedLoser;

  const [entity, setEntity] = useState<Entity | null>(null);
  const [contacts, setContacts] = useState<EntityContactLink[]>([]);
  const [contactsError, setContactsError] = useState(false);
  const [definitions, setDefinitions] = useState<FieldDefinition[]>([]);
  const [values, setValues] = useState<FieldValue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notFound, setNotFound] = useState(false);
  const [deleted, setDeleted] = useState<Entity | null>(null);
  const [editing, setEditing] = useState(false);
  const [merging, setMerging] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [restoring, setRestoring] = useState(false);

  useDocumentTitle(entity?.name ?? t('entities.title'));

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    setNotFound(false);
    try {
      const e = await getEntity(id);
      setEntity(e);
      setDeleted(null);
      const [people, defs, vals] = await Promise.allSettled([
        listEntityContacts(id),
        getFieldDefinitions(100),
        listEntityFieldValues(id),
      ]);
      setContactsError(people.status === 'rejected');
      setContacts(people.status === 'fulfilled' ? people.value : []);
      setDefinitions(defs.status === 'fulfilled' ? defs.value.field_definitions || [] : []);
      setValues(vals.status === 'fulfilled' ? vals.value : []);
    } catch (err) {
      setEntity(null);
      if (err instanceof ApiError && err.status === 404) setNotFound(true);
      else setError(getErrorMessage(err) || t('entities.detail.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const kindDefinitions = useMemo(
    () => (entity ? definitions.filter((d) => d.target === entity.kind) : []),
    [definitions, entity],
  );
  const valuesByDefinition = useMemo(
    () => new Map(values.map((v) => [v.field_definition_id, v.value])),
    [values],
  );

  const saveFieldValue = async (definitionId: string, value: unknown) => {
    try {
      setValues(
        await replaceEntityFieldValues(
          id,
          fieldValueInputsWith(valuesByDefinition, definitionId, value),
        ),
      );
      showSuccess(t('entities.detail.customFieldsSaved'));
    } catch (err) {
      showError(getErrorMessage(err));
      throw err;
    }
  };

  const restore = async (restoreId: string, name: string) => {
    setRestoring(true);
    try {
      const result = await restoreEntity(restoreId);
      showSuccess(t('entities.deleted.restored', { name }));
      if (result.same_name_live) {
        showWarning(
          t('entities.deleted.sameNameLive', {
            kind: t(ENTITY_KIND_KEYS[result.entity.kind]).toLowerCase(),
          }),
        );
      }
      if (restoreId === id) {
        await load();
      } else {
        void navigate(`/entities/${restoreId}`, { replace: true });
      }
    } catch (err) {
      showError(getErrorMessage(err) || t('entities.deleted.restoreFailed'));
    } finally {
      setRestoring(false);
    }
  };

  if (loading && !entity && !deleted) {
    return (
      <Box sx={{ p: 4, textAlign: 'center' }}>
        <CircularProgress aria-label={t('common.loading')} />
      </Box>
    );
  }

  const back = (
    <Button component={Link} to="/entities" startIcon={<ArrowBackIcon />} sx={{ mb: 1 }}>
      {t('entities.detail.back')}
    </Button>
  );

  if (deleted) {
    return (
      <Box sx={{ maxWidth: 900, mx: 'auto', mt: 1, px: 2, pb: 4 }}>
        {back}
        <Alert
          severity="info"
          action={
            <Button
              color="inherit"
              size="small"
              disabled={restoring}
              onClick={() => void restore(deleted.id, deleted.name)}
            >
              {t('entities.deleted.restore')}
            </Button>
          }
        >
          <AlertTitle>{t('entities.deleted.title', { name: deleted.name })}</AlertTitle>
          {t('entities.deleted.body')}
        </Alert>
      </Box>
    );
  }

  if (notFound || error || !entity) {
    return (
      <Box sx={{ maxWidth: 900, mx: 'auto', mt: 1, px: 2, pb: 4 }}>
        {back}
        {error ? (
          <LoadError message={error} onRetry={() => void load()} />
        ) : (
          <Alert severity="warning">{t('entities.detail.notFound')}</Alert>
        )}
      </Box>
    );
  }

  const form = coreToForm(entity.kind, entity.core) as unknown as Record<string, string>;
  const detailRows = detailFields(entity.kind)
    .map(({ key, label }) => ({ key, label: t(label), value: form[key] }))
    .filter((r) => r.value);

  return (
    <Box sx={{ maxWidth: 900, mx: 'auto', mt: 1, px: 2, pb: 4 }}>
      {back}
      {mergedLoser && (
        <Alert
          severity="success"
          sx={{ mb: 2 }}
          action={
            <Button
              color="inherit"
              size="small"
              disabled={restoring}
              onClick={() => void restore(mergedLoser.id, mergedLoser.name)}
            >
              {t('entities.merge.undo', { name: mergedLoser.name })}
            </Button>
          }
        >
          <AlertTitle>{t('entities.merge.undoTitle', { name: mergedLoser.name })}</AlertTitle>
          {t('entities.merge.undoBody')}
        </Alert>
      )}
      <PageHeader
        title={entity.name}
        actions={
          <>
            <Button variant="outlined" onClick={() => setEditing(true)}>
              {t('entities.detail.edit')}
            </Button>
            <Button variant="outlined" onClick={() => setMerging(true)}>
              {t('entities.detail.merge')}
            </Button>
            <Button variant="outlined" color="error" onClick={() => setDeleting(true)}>
              {t('entities.detail.delete')}
            </Button>
          </>
        }
      />
      <Stack direction="row" spacing={1} sx={{ mb: 2, alignItems: 'center', flexWrap: 'wrap' }}>
        <Chip size="small" label={t(ENTITY_KIND_KEYS[entity.kind])} />
        {entity.sensitivity !== 'normal' && (
          <Chip
            size="small"
            color="warning"
            label={t(ENTITY_SENSITIVITY_KEYS[entity.sensitivity])}
          />
        )}
        <Typography variant="body2" color="text.secondary">
          {t('entities.detail.linkedCount', { count: entity.link_count ?? contacts.length })}
        </Typography>
      </Stack>

      <Card
        variant="outlined"
        sx={{ mb: 2 }}
        component="section"
        aria-label={t('entities.detail.details')}
      >
        <CardContent>
          {detailRows.length === 0 ? (
            <Typography color="text.secondary">{t('entities.detail.noDetails')}</Typography>
          ) : (
            <Box
              component="dl"
              sx={{
                m: 0,
                display: 'grid',
                gridTemplateColumns: 'max-content 1fr',
                gap: 1,
                columnGap: 3,
              }}
            >
              {detailRows.map((r) => (
                <Box key={r.key} sx={{ display: 'contents' }}>
                  <Typography component="dt" color="text.secondary">
                    {r.label}
                  </Typography>
                  <Typography
                    component="dd"
                    sx={{ m: 0, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}
                  >
                    {r.value}
                  </Typography>
                </Box>
              ))}
            </Box>
          )}
        </CardContent>
      </Card>

      <Card
        variant="outlined"
        sx={{ mb: 2 }}
        component="section"
        aria-label={t('entities.detail.customFields')}
      >
        <CardContent>
          <Typography variant="h2" sx={{ fontSize: '1.125rem', mb: 1 }}>
            {t('entities.detail.customFields')}
          </Typography>
          {kindDefinitions.length === 0 ? (
            <Typography color="text.secondary">{t('entities.detail.noCustomFields')}</Typography>
          ) : (
            <CustomFieldGroups
              definitions={kindDefinitions}
              valuesByDefinition={valuesByDefinition}
              onSave={saveFieldValue}
            />
          )}
        </CardContent>
      </Card>

      <Card variant="outlined" component="section" aria-label={t('entities.people.title')}>
        <CardContent>
          <Typography variant="h2" sx={{ fontSize: '1.125rem', mb: 1 }}>
            {t('entities.people.title')}
          </Typography>
          {contactsError ? (
            <Alert severity="error">{t('entities.people.loadFailed')}</Alert>
          ) : contacts.length === 0 ? (
            <Typography color="text.secondary">{t('entities.people.empty')}</Typography>
          ) : (
            <Stack component="ul" spacing={1} sx={{ listStyle: 'none', p: 0, m: 0 }}>
              {contacts.map((c) => {
                const key = roleKey(entity.kind, c.role);
                const role = key ? t(key) : c.role;
                const period = formatLinkPeriod(c.period, t);
                return (
                  <Box
                    component="li"
                    key={c.link_id}
                    sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}
                  >
                    <MuiLink component={Link} to={`/contacts/${c.contact_id}`}>
                      {`${c.firstname} ${c.lastname}`.trim()}
                    </MuiLink>
                    <Typography variant="body2" color="text.secondary">
                      {role}
                    </Typography>
                    {c.role_label && (
                      <Typography variant="body2" color="text.secondary">
                        {c.role_label}
                      </Typography>
                    )}
                    {period && (
                      <Typography variant="body2" color="text.secondary">
                        {period}
                      </Typography>
                    )}
                    {c.is_primary && (
                      <Chip size="small" color="primary" label={t('entities.people.primary')} />
                    )}
                    {c.sensitivity !== 'normal' && (
                      <Chip
                        size="small"
                        color="warning"
                        label={t(ENTITY_SENSITIVITY_KEYS[c.sensitivity])}
                      />
                    )}
                  </Box>
                );
              })}
            </Stack>
          )}
        </CardContent>
      </Card>

      <EntityFormDialog
        open={editing}
        entity={entity}
        onClose={() => setEditing(false)}
        onSaved={(saved) => {
          setEditing(false);
          setEntity((prev) => ({
            ...saved,
            link_count: prev?.link_count,
            projection_count: prev?.projection_count,
          }));
          showSuccess(t('entities.form.updated', { name: saved.name }));
        }}
      />
      {merging && (
        <EntityMergeDialog
          open
          loserId={entity.id}
          onClose={() => setMerging(false)}
          onMerged={(result, loser) => {
            setMerging(false);
            showSuccess(t('entities.merge.done', { name: result.survivor.name }));
            void navigate(`/entities/${result.survivor.id}`, {
              state: { mergedLoser: { id: loser.id, name: loser.name } },
            });
          }}
        />
      )}
      <EntityDeleteDialog
        open={deleting}
        entity={entity}
        contacts={contacts}
        onClose={() => setDeleting(false)}
        onDeleted={() => {
          setDeleting(false);
          setDeleted(entity);
        }}
      />
    </Box>
  );
}

// Which core fields each kind shows on the detail card (form key -> label key).
function detailFields(kind: Entity['kind']): { key: string; label: string }[] {
  if (kind === 'organisation') {
    return [
      { key: 'url', label: 'entities.fields.url' },
      { key: 'sortAs', label: 'entities.fields.sortAs' },
      { key: 'unitsText', label: 'entities.fields.units' },
      { key: 'notes', label: 'entities.fields.notes' },
    ];
  }
  if (kind === 'place') {
    return [
      { key: 'street', label: 'entities.fields.street' },
      { key: 'city', label: 'entities.fields.city' },
      { key: 'region', label: 'entities.fields.region' },
      { key: 'postal', label: 'entities.fields.postal' },
      { key: 'country', label: 'entities.fields.country' },
      { key: 'notes', label: 'entities.fields.notes' },
    ];
  }
  return [
    { key: 'species', label: 'entities.fields.species' },
    { key: 'breed', label: 'entities.fields.breed' },
    { key: 'birth', label: 'entities.fields.birth' },
    { key: 'death', label: 'entities.fields.death' },
    { key: 'notes', label: 'entities.fields.notes' },
  ];
}
