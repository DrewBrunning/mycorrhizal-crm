import DomainIcon from '@mui/icons-material/Domain';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  MenuItem,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  createEntitiesFromContacts,
  ENTITY_LINK_ROLES,
  type EntityFromContactsSkip,
  type EntityValueGroup,
  listEntitySuggestions,
} from '../api/entities';
import { useSnackbar } from '../context/SnackbarContext';
import { ENTITY_KIND_PLURAL_KEYS, roleKey } from '../utils/entityLabels';
import { getErrorMessage } from '../utils/errorHandler';
import AppDialog from './AppDialog';

type HelperKind = 'organisation' | 'place';
const HELPER_KINDS: HelperKind[] = ['organisation', 'place'];

interface GroupState {
  selected: boolean;
  name: string;
  role: string;
  primary: boolean;
}

const defaultState = (g: EntityValueGroup, kind: HelperKind): GroupState => ({
  selected: false,
  name: g.name,
  role: ENTITY_LINK_ROLES[kind][0],
  primary: false,
});

// Settings card for the optional, user-triggered "create entities from my
// contacts" helper (ADR 0043 decision 9). Nothing runs until the user opens the
// wizard; it never edits or deletes a contact value.
export default function EntitiesFromContactsSettings() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <DomainIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('entities.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          {t('entities.fromContacts.description')}
        </Typography>
        <Button variant="outlined" onClick={() => setOpen(true)}>
          {t('entities.fromContacts.open')}
        </Button>
      </CardContent>
      {open && <FromContactsDialog onClose={() => setOpen(false)} />}
    </Card>
  );
}

function FromContactsDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const [kind, setKind] = useState<HelperKind>('organisation');
  const [groups, setGroups] = useState<EntityValueGroup[]>([]);
  const [state, setState] = useState<Record<string, GroupState>>({});
  const [next, setNext] = useState('');
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);
  const [skipped, setSkipped] = useState<EntityFromContactsSkip[]>([]);
  const [createdCount, setCreatedCount] = useState<number | null>(null);
  const requestRef = useRef(0);

  const load = useCallback(async (k: HelperKind, cursor?: string) => {
    const requestId = ++requestRef.current;
    setLoading(true);
    setError('');
    try {
      const res = await listEntitySuggestions({ kind: k, mode: 'contact_values', cursor });
      if (requestRef.current !== requestId) return;
      setGroups((prev) => (cursor ? [...prev, ...res.groups] : res.groups));
      setState((prev) => {
        const merged = cursor ? { ...prev } : {};
        for (const g of res.groups) merged[g.key] = defaultState(g, k);
        return merged;
      });
      setNext(res.next_cursor);
      setLoaded(true);
    } catch (err) {
      if (requestRef.current === requestId) setError(getErrorMessage(err));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, []);

  useEffect(() => {
    setLoaded(false);
    setGroups([]);
    setSkipped([]);
    setCreatedCount(null);
    void load(kind);
  }, [kind, load]);

  const patch = (key: string, p: Partial<GroupState>) =>
    setState((prev) => ({ ...prev, [key]: { ...prev[key], ...p } }));

  const chosen = groups.filter((g) => state[g.key]?.selected);

  const handleCreate = async () => {
    setCreating(true);
    setError('');
    try {
      const res = await createEntitiesFromContacts({
        kind,
        groups: chosen.map((g) => ({
          key: g.key,
          name: state[g.key].name.trim() || g.name,
          role: state[g.key].role,
          set_primary: state[g.key].primary,
          contact_ids: g.contacts.map((c) => c.contact_id),
        })),
      });
      setCreatedCount(res.created.length);
      setSkipped(res.skipped);
      if (res.created.length > 0) {
        showSuccess(
          kind === 'place'
            ? t('entities.fromContacts.createdPlace', { count: res.created.length })
            : t('entities.fromContacts.createdOrganisation', { count: res.created.length }),
        );
      }
      // Resumable: re-scan so only the still-unmatched values remain.
      await load(kind);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  const createLabel =
    kind === 'place'
      ? t('entities.fromContacts.createPlace', { count: chosen.length })
      : t('entities.fromContacts.createOrganisation', { count: chosen.length });
  const hasPrimaryKey =
    kind === 'place'
      ? 'entities.fromContacts.hasPrimaryPlace'
      : 'entities.fromContacts.hasPrimaryOrganisation';

  return (
    <AppDialog
      open
      onClose={onClose}
      maxWidth="md"
      fullWidth
      aria-labelledby="entities-from-contacts-title"
    >
      <DialogTitle id="entities-from-contacts-title">
        {t('entities.fromContacts.title')}
      </DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {t('entities.fromContacts.intro')}
        </Typography>
        <Tabs
          value={kind}
          onChange={(_, v: HelperKind) => setKind(v)}
          aria-label={t('entities.tabsLabel')}
          sx={{ mb: 2 }}
        >
          {HELPER_KINDS.map((k) => (
            <Tab key={k} value={k} label={t(ENTITY_KIND_PLURAL_KEYS[k])} />
          ))}
        </Tabs>
        {error && (
          <Alert severity="error" sx={{ mb: 1 }}>
            {error}
          </Alert>
        )}
        {createdCount !== null && createdCount > 0 && (
          <Alert severity="success" sx={{ mb: 1 }}>
            {kind === 'place'
              ? t('entities.fromContacts.createdPlace', { count: createdCount })
              : t('entities.fromContacts.createdOrganisation', { count: createdCount })}
          </Alert>
        )}
        {skipped.length > 0 && (
          <Alert severity="warning" sx={{ mb: 1 }}>
            <Stack component="ul" sx={{ m: 0, pl: 2 }}>
              {skipped.map((s) => (
                <li key={`${s.key}:${s.contact_id ?? ''}:${s.reason}`}>
                  {skipText(t, s, groups.find((g) => g.key === s.key)?.name ?? s.key)}
                </li>
              ))}
            </Stack>
          </Alert>
        )}
        {loaded && groups.length === 0 && !loading && (
          <Typography>{t('entities.fromContacts.none')}</Typography>
        )}
        <Stack component="ul" spacing={2} sx={{ listStyle: 'none', p: 0, m: 0 }}>
          {groups.map((g) => {
            const gs = state[g.key] ?? defaultState(g, kind);
            const withPrimary = g.contacts.filter((c) => c.has_primary).length;
            return (
              <Box
                component="li"
                key={g.key}
                sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Checkbox
                    checked={gs.selected}
                    onChange={(e) => patch(g.key, { selected: e.target.checked })}
                    slotProps={{
                      input: { 'aria-label': t('entities.fromContacts.include', { name: g.name }) },
                    }}
                  />
                  <Typography sx={{ fontWeight: 500 }}>
                    {t('entities.fromContacts.groupTitle', {
                      name: g.name,
                      count: g.contacts.length,
                    })}
                  </Typography>
                </Box>
                <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap', ml: 1.5, mb: 1 }}>
                  {g.variants.map((v) => (
                    <Chip key={v} size="small" variant="outlined" label={v} />
                  ))}
                </Box>
                {withPrimary > 0 && (
                  <Typography variant="body2" color="text.secondary" sx={{ ml: 1.5, mb: 1 }}>
                    {t(hasPrimaryKey, { count: withPrimary })}
                  </Typography>
                )}
                {gs.selected && (
                  <Box
                    sx={{
                      display: 'flex',
                      gap: 2,
                      flexWrap: 'wrap',
                      alignItems: 'center',
                      ml: 1.5,
                    }}
                  >
                    <TextField
                      size="small"
                      label={t('entities.fromContacts.nameOf', { name: g.name })}
                      value={gs.name}
                      onChange={(e) => patch(g.key, { name: e.target.value })}
                      slotProps={{ htmlInput: { maxLength: 200 } }}
                    />
                    <TextField
                      select
                      size="small"
                      label={t('entities.fromContacts.roleFor', { name: g.name })}
                      value={gs.role}
                      onChange={(e) => patch(g.key, { role: e.target.value })}
                      sx={{ minWidth: 160 }}
                    >
                      {ENTITY_LINK_ROLES[kind].map((r) => (
                        <MenuItem key={r} value={r}>
                          {t(roleKey(kind, r) ?? 'entities.roles.organisation.other')}
                        </MenuItem>
                      ))}
                    </TextField>
                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={gs.primary}
                          onChange={(e) => patch(g.key, { primary: e.target.checked })}
                          slotProps={{
                            input: {
                              'aria-label': t('entities.fromContacts.primaryFor', { name: g.name }),
                            },
                          }}
                        />
                      }
                      label={t('entities.fromContacts.primary')}
                    />
                  </Box>
                )}
              </Box>
            );
          })}
        </Stack>
        {next && (
          <Button onClick={() => void load(kind, next)} disabled={loading} sx={{ mt: 1 }}>
            {t('common.loadMore')}
          </Button>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.close')}</Button>
        <Button
          variant="contained"
          onClick={() => void handleCreate()}
          disabled={creating || chosen.length === 0}
        >
          {createLabel}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}

function skipText(
  t: (key: string, opts?: Record<string, unknown>) => string,
  s: EntityFromContactsSkip,
  name: string,
): string {
  switch (s.reason) {
    case 'group_not_found':
      return t('entities.fromContacts.skip.group_not_found', { name });
    case 'contact_not_in_group':
      return t('entities.fromContacts.skip.contact_not_in_group', { name });
    case 'name_invalid':
      return t('entities.fromContacts.skip.name_invalid', { name });
    case 'core_invalid':
      return t('entities.fromContacts.skip.core_invalid', { name });
  }
}
