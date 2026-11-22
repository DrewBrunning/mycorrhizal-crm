import SearchIcon from '@mui/icons-material/Search';
import {
  Box,
  Chip,
  IconButton,
  InputAdornment,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  Paper,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink, useSearchParams } from 'react-router';
import { type SearchResult, searchAll } from './api/search';
import LoadError from './components/LoadError';
import { useDateFormat } from './DateFormatProvider';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { ENTITY_KIND_KEYS, ENTITY_SENSITIVITY_KEYS } from './utils/entityLabels';
import { getErrorMessage } from './utils/errorHandler';
import { calendarDateOnly } from './utils/localDate';
import {
  activityHref,
  contactHref,
  entityHref,
  noteHref,
  type SearchTab,
} from './utils/searchLinks';
import { matchReason } from './utils/searchMatch';

// #2174: the full results page behind the top-bar search — one tab per
// category. It asks for the server's maximum page (50) per category; when a
// category is full it says so rather than implying the list is complete.
const PAGE_LIMIT = 50;
const MIN_QUERY_LENGTH = 2;
const TABS: SearchTab[] = ['contacts', 'notes', 'activities', 'entities'];

function explicitTab(raw: string | null): SearchTab | null {
  return TABS.includes(raw as SearchTab) ? (raw as SearchTab) : null;
}

export default function SearchResultsPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('globalSearch.pageTitle'));
  const { formatDate } = useDateFormat();
  const [searchParams, setSearchParams] = useSearchParams();
  const query = (searchParams.get('q') || '').trim();
  const requestedTab = explicitTab(searchParams.get('tab'));
  const tooShort = [...query].length < MIN_QUERY_LENGTH;

  const [result, setResult] = useState<SearchResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const requestRef = useRef(0);

  const load = useCallback(async () => {
    const requestId = ++requestRef.current;
    if ([...query].length < MIN_QUERY_LENGTH) {
      setResult(null);
      setError(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const r = await searchAll(query, PAGE_LIMIT);
      if (requestRef.current === requestId) setResult(r);
    } catch (err) {
      if (requestRef.current === requestId) {
        setResult(null);
        setError(getErrorMessage(err));
      }
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [query]);

  useEffect(() => {
    void load();
  }, [load]);

  const counts: Record<SearchTab, number> = {
    contacts: result?.contacts.length ?? 0,
    notes: result?.notes.length ?? 0,
    activities: result?.activities.length ?? 0,
    entities: result?.entities.length ?? 0,
  };

  // #2698: with no explicit tab, open the first category that has hits (an
  // explicit ?tab= is always honoured; all-empty falls back to Contacts).
  const tab: SearchTab = requestedTab ?? TABS.find((k) => counts[k] > 0) ?? 'contacts';

  // #2692: the AppBar search is absent on phones, so the page has its own field.
  const [draft, setDraft] = useState(query);
  useEffect(() => setDraft(query), [query]);
  const fieldRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!query) fieldRef.current?.focus();
  }, [query]);
  const submitDraft = (e: FormEvent) => {
    e.preventDefault();
    const term = draft.trim();
    if (term) setSearchParams({ q: term });
  };

  const categoryName = (k: SearchTab) =>
    k === 'contacts'
      ? t('globalSearch.groupContacts')
      : k === 'notes'
        ? t('globalSearch.groupNotes')
        : k === 'activities'
          ? t('globalSearch.groupActivities')
          : t('globalSearch.groupEntities');

  const tabLabel = (k: SearchTab) =>
    result ? `${categoryName(k)} (${counts[k]})` : categoryName(k);

  const emptyText = (k: SearchTab) =>
    k === 'contacts'
      ? t('globalSearch.emptyContacts', { query })
      : k === 'notes'
        ? t('globalSearch.emptyNotes', { query })
        : k === 'activities'
          ? t('globalSearch.emptyActivities', { query })
          : t('globalSearch.emptyEntities', { query });

  const empty = (k: SearchTab) => (
    <Paper sx={{ p: 3, textAlign: 'center' }}>
      <Typography sx={{ color: 'text.secondary' }}>{emptyText(k)}</Typography>
    </Paper>
  );

  const row = (
    key: string | number,
    to: string,
    primary: string,
    secondary?: string,
    chip?: string,
    // Issue #2744: why the hit matched, on its own line so a narrow screen keeps the title wide.
    reason?: string,
  ) => (
    <ListItem key={key} disablePadding>
      <ListItemButton component={RouterLink} to={to}>
        <ListItemText
          primary={primary}
          secondary={
            secondary || reason ? (
              <>
                {secondary}
                {reason && (
                  <Box component="span" sx={{ display: 'block', fontStyle: 'italic' }}>
                    {reason}
                  </Box>
                )}
              </>
            ) : undefined
          }
          slotProps={{
            primary: { sx: { overflowWrap: 'anywhere' } },
          }}
        />
        {chip && (
          <Chip size="small" variant="outlined" label={chip} sx={{ ml: 2, flexShrink: 0 }} />
        )}
      </ListItemButton>
    </ListItem>
  );

  const renderTab = () => {
    if (!result) return null;
    if (counts[tab] === 0) return empty(tab);
    return (
      <Paper>
        <List>
          {tab === 'contacts' &&
            result.contacts.map((c) =>
              row(
                c.id,
                contactHref(c.id),
                `${c.firstname} ${c.lastname}`.trim() || c.fn || '',
                [c.org, c.primary_email].filter(Boolean).join(' · ') || undefined,
                undefined,
                matchReason(t, c.matched_on),
              ),
            )}
          {tab === 'notes' &&
            result.notes.map((n) =>
              row(
                n.id,
                noteHref(n, query),
                n.content,
                [
                  n.contact_name || t('globalSearch.unfiled'),
                  formatDate(calendarDateOnly(n.date)),
                ].join(' · '),
              ),
            )}
          {tab === 'activities' &&
            result.activities.map((a) =>
              row(
                a.id,
                activityHref(query),
                a.title || a.description || '',
                [a.location, formatDate(calendarDateOnly(a.date))].filter(Boolean).join(' · '),
              ),
            )}
          {tab === 'entities' &&
            result.entities.map((e) =>
              row(
                e.id,
                entityHref(e.id),
                e.name,
                e.sensitivity !== 'normal' ? t(ENTITY_SENSITIVITY_KEYS[e.sensitivity]) : undefined,
                t(ENTITY_KIND_KEYS[e.kind]),
              ),
            )}
        </List>
        {counts[tab] >= PAGE_LIMIT && (
          <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', p: 2 }}>
            {t('globalSearch.capped', { limit: PAGE_LIMIT })}
          </Typography>
        )}
      </Paper>
    );
  };

  return (
    <Box>
      <Typography variant="h5" component="h1" gutterBottom>
        {t('globalSearch.pageTitle')}
      </Typography>
      <Box component="form" role="search" onSubmit={submitDraft} sx={{ mb: 2 }}>
        <TextField
          type="search"
          fullWidth
          size="small"
          label={t('globalSearch.label')}
          placeholder={t('globalSearch.placeholder')}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          inputRef={fieldRef}
          slotProps={{
            input: {
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton type="submit" size="small" aria-label={t('app.submitSearch')}>
                    <SearchIcon />
                  </IconButton>
                </InputAdornment>
              ),
            },
          }}
        />
      </Box>
      {tooShort ? (
        <Typography sx={{ color: 'text.secondary' }}>
          {t('contacts.searchMinLengthHint')}
        </Typography>
      ) : (
        <>
          <Tabs
            value={tab}
            onChange={(_, v: SearchTab) => {
              const next = new URLSearchParams(searchParams);
              next.set('tab', v); // always explicit: Contacts may be the empty tab
              setSearchParams(next, { replace: true });
            }}
            aria-label={t('globalSearch.tabsLabel')}
            variant="scrollable"
            scrollButtons="auto"
            allowScrollButtonsMobile
            sx={{ mb: 2 }}
          >
            {TABS.map((k) => (
              <Tab key={k} value={k} label={tabLabel(k)} />
            ))}
          </Tabs>
          {error ? (
            <LoadError message={error} onRetry={() => void load()} />
          ) : loading && !result ? null : (
            renderTab()
          )}
        </>
      )}
    </Box>
  );
}
