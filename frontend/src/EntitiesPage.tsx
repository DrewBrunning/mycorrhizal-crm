import AddIcon from '@mui/icons-material/Add';
import {
  Box,
  Button,
  List,
  ListItemButton,
  ListItemText,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useSearchParams } from 'react-router';
import { ENTITY_KINDS, type EntityKind, type EntitySuggestionPair } from './api/entities';
import EntityFormDialog from './components/EntityFormDialog';
import EntityMergeDialog from './components/EntityMergeDialog';
import EntitySuggestionsPanel, { pairKey } from './components/EntitySuggestionsPanel';
import LoadError from './components/LoadError';
import PageHeader from './components/PageHeader';
import { useSnackbar } from './context/SnackbarContext';
import { useDebouncedValue } from './hooks/useDebounce';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useEntityList } from './hooks/useEntityList';
import { coreSummary } from './utils/entityCore';
import {
  ENTITY_EMPTY_KEYS,
  ENTITY_KIND_PLURAL_KEYS,
  ENTITY_NEW_KEYS,
  ENTITY_SENSITIVITY_KEYS,
} from './utils/entityLabels';

function kindFromParam(raw: string | null): EntityKind {
  return ENTITY_KINDS.find((k) => k === raw) ?? 'organisation';
}

// Entities (issue #2301, ADR 0043): the reusable organisations, places and pets
// that several contacts can link to. One list per kind.
export default function EntitiesPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('entities.title'));
  const { showSuccess } = useSnackbar();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const kind = kindFromParam(params.get('kind'));
  const [search, setSearch] = useState('');
  const debounced = useDebouncedValue(search);
  const { entities, loading, error, hasMore, loadMore, refresh } = useEntityList(kind, debounced);
  const [creating, setCreating] = useState(false);
  const [merging, setMerging] = useState<EntitySuggestionPair | null>(null);
  const [resolved, setResolved] = useState<Set<string>>(new Set());

  const selectKind = (next: EntityKind) => {
    setSearch('');
    setParams(next === 'organisation' ? {} : { kind: next }, { replace: true });
  };

  return (
    <Box sx={{ maxWidth: 900, mx: 'auto', mt: 1, px: 2, pb: 4 }}>
      <PageHeader
        title={t('entities.title')}
        actions={
          <Button variant="contained" startIcon={<AddIcon />} onClick={() => setCreating(true)}>
            {t(ENTITY_NEW_KEYS[kind])}
          </Button>
        }
      />
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('entities.description')}
      </Typography>
      <Tabs
        value={kind}
        onChange={(_, v: EntityKind) => selectKind(v)}
        aria-label={t('entities.tabsLabel')}
        variant="scrollable"
        sx={{ mb: 2 }}
      >
        {ENTITY_KINDS.map((k) => (
          <Tab key={k} value={k} label={t(ENTITY_KIND_PLURAL_KEYS[k])} />
        ))}
      </Tabs>
      <TextField
        label={t('entities.search')}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        size="small"
        fullWidth
        sx={{ mb: 2 }}
      />
      {error ? (
        <LoadError message={error} onRetry={() => void refresh()} />
      ) : entities.length === 0 ? (
        !loading && (
          <Typography sx={{ py: 2 }}>
            {debounced.trim() ? t('entities.noResults') : t(ENTITY_EMPTY_KEYS[kind])}
          </Typography>
        )
      ) : (
        <List disablePadding aria-label={t(ENTITY_KIND_PLURAL_KEYS[kind])}>
          {entities.map((e) => (
            <ListItemButton
              key={e.id}
              component={Link}
              to={`/entities/${e.id}`}
              divider
              sx={{ borderRadius: 1 }}
            >
              <ListItemText
                primary={e.name}
                secondary={
                  [
                    coreSummary(e),
                    e.sensitivity !== 'normal' ? t(ENTITY_SENSITIVITY_KEYS[e.sensitivity]) : '',
                  ]
                    .filter(Boolean)
                    .join(' · ') || undefined
                }
              />
            </ListItemButton>
          ))}
        </List>
      )}
      {hasMore && (
        <Button onClick={() => void loadMore()} disabled={loading} sx={{ mt: 1 }}>
          {t('common.loadMore')}
        </Button>
      )}

      <EntitySuggestionsPanel kind={kind} onMerge={setMerging} resolvedPairKeys={resolved} />

      <EntityFormDialog
        open={creating}
        kind={kind}
        onClose={() => setCreating(false)}
        onSaved={(saved) => {
          setCreating(false);
          showSuccess(t('entities.form.created', { name: saved.name }));
          void navigate(`/entities/${saved.id}`);
        }}
      />
      {merging && (
        <EntityMergeDialog
          open
          loserId={merging.b.id}
          survivorId={merging.a.id}
          onClose={() => setMerging(null)}
          onMerged={(result) => {
            setResolved((prev) => new Set(prev).add(pairKey(merging)));
            setMerging(null);
            showSuccess(t('entities.merge.done', { name: result.survivor.name }));
            void refresh();
          }}
        />
      )}
    </Box>
  );
}
