import ClearIcon from '@mui/icons-material/Clear';
import SearchIcon from '@mui/icons-material/Search';
import {
  Autocomplete,
  Box,
  IconButton,
  InputAdornment,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router';
import { type SearchResult, searchAll } from '../api/search';
import { ENTITY_KIND_KEYS, ENTITY_SENSITIVITY_KEYS } from '../utils/entityLabels';
import {
  activityHref,
  contactHref,
  entityHref,
  noteHref,
  searchResultsHref,
} from '../utils/searchLinks';

// #2174/#2472: the top-bar search covers contacts, notes, activities and entities. The
// dropdown shows the top DROPDOWN_LIMIT hits per category and a last row that
// opens the full results page (one tab per category).
const DROPDOWN_LIMIT = 3;
const MIN_QUERY_LENGTH = 2;
const DEBOUNCE_MS = 300;

type Kind = 'contacts' | 'notes' | 'activities' | 'entities' | 'all' | 'none';

interface Option {
  key: string;
  kind: Kind;
  primary: string;
  secondary?: string;
  to: string;
}

function toOptions(
  result: SearchResult,
  query: string,
  t: (key: string, opts?: Record<string, unknown>) => string,
): Option[] {
  const options: Option[] = [];
  for (const c of result.contacts) {
    options.push({
      key: `c${c.id}`,
      kind: 'contacts',
      primary: `${c.firstname} ${c.lastname}`.trim() || c.fn || '',
      secondary: c.primary_email || c.org,
      to: contactHref(c.id),
    });
  }
  for (const n of result.notes) {
    options.push({
      key: `n${n.id}`,
      kind: 'notes',
      primary: n.content,
      secondary: n.contact_name,
      to: noteHref(n, query),
    });
  }
  for (const a of result.activities) {
    options.push({
      key: `a${a.id}`,
      kind: 'activities',
      primary: a.title || a.description || '',
      secondary: a.location,
      to: activityHref(query),
    });
  }
  for (const e of result.entities) {
    options.push({
      key: `e${e.id}`,
      kind: 'entities',
      primary: e.name,
      secondary: [
        t(ENTITY_KIND_KEYS[e.kind]),
        e.sensitivity !== 'normal' ? t(ENTITY_SENSITIVITY_KEYS[e.sensitivity]) : '',
      ]
        .filter(Boolean)
        .join(' · '),
      to: entityHref(e.id),
    });
  }
  if (options.length === 0) {
    options.push({
      key: 'none',
      kind: 'none',
      primary: t('globalSearch.noResults', { query }),
      to: searchResultsHref(query),
    });
  } else {
    options.push({
      key: 'all',
      kind: 'all',
      primary: t('globalSearch.seeAll', { query }),
      to: searchResultsHref(query),
    });
  }
  return options;
}

export default function GlobalSearch() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [query, setQuery] = useState('');
  const [options, setOptions] = useState<Option[]>([]);
  const [loading, setLoading] = useState(false);
  const highlighted = useRef<Option | null>(null);

  // Stale-response guard (issue #960): a slow response for an abandoned query
  // must never overwrite the results (or spinner) for a newer one.
  const requestRef = useRef(0);

  useEffect(() => {
    const requestId = ++requestRef.current;
    const term = query.trim();
    if ([...term].length < MIN_QUERY_LENGTH) {
      setOptions([]);
      return;
    }
    const timer = setTimeout(() => {
      void (async () => {
        setLoading(true);
        try {
          const result = await searchAll(term, DROPDOWN_LIMIT);
          if (requestRef.current !== requestId) return;
          setOptions(toOptions(result, term, t));
        } catch (err) {
          if (requestRef.current !== requestId) return;
          console.error('Search error:', err);
          setOptions([]);
        } finally {
          if (requestRef.current === requestId) setLoading(false);
        }
      })();
    }, DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [query, t]);

  // Any navigation (a result click, the logo, the nav drawer) starts the box
  // over, so a stale query never lingers over an unrelated page.
  useEffect(() => reset(), [pathname]);

  const reset = () => {
    setQuery('');
    setOptions([]);
  };

  const submit = () => {
    const term = query.trim();
    if (!term) return;
    void navigate(searchResultsHref(term));
    reset();
  };

  const groupLabel = (kind: string) => {
    switch (kind) {
      case 'contacts':
        return t('globalSearch.groupContacts');
      case 'notes':
        return t('globalSearch.groupNotes');
      case 'activities':
        return t('globalSearch.groupActivities');
      case 'entities':
        return t('globalSearch.groupEntities');
      default:
        return '';
    }
  };

  return (
    <Autocomplete<Option, false, false, true>
      freeSolo
      size="small"
      options={options}
      // The server already ranked and limited the hits; MUI's own text filter
      // would drop FTS matches whose label does not contain the typed text
      // (a stemmed or address match) and the synthetic rows.
      filterOptions={(o) => o}
      groupBy={(o) => o.kind}
      getOptionLabel={(o) => (typeof o === 'string' ? o : '')}
      isOptionEqualToValue={(a, b) => typeof b !== 'string' && a.key === b.key}
      loading={loading}
      inputValue={query}
      onInputChange={(_, value, reason) => {
        if (reason === 'reset') return; // selecting an option must not rewrite the box
        setQuery(value);
      }}
      onHighlightChange={(_, option) => {
        highlighted.current = option;
      }}
      onChange={(_, value) => {
        if (value && typeof value !== 'string') {
          void navigate(value.to);
          reset();
        }
      }}
      onKeyDown={(event) => {
        // With a highlighted row MUI selects it; otherwise Enter submits the
        // typed text to the results page.
        if (event.key === 'Enter' && !highlighted.current) submit();
      }}
      renderGroup={(params) => (
        <li key={params.key}>
          {groupLabel(params.group) && (
            <Typography
              variant="overline"
              component="div"
              sx={{ px: 2, pt: 1, color: 'text.secondary', lineHeight: 2 }}
            >
              {groupLabel(params.group)}
            </Typography>
          )}
          <ul style={{ padding: 0 }}>{params.children}</ul>
        </li>
      )}
      renderOption={(props, option) => {
        const { key: _k, ...liProps } = props as typeof props & { key?: string };
        return (
          <li {...liProps} key={option.key}>
            <Box sx={{ minWidth: 0, width: '100%' }}>
              <Typography
                variant="body2"
                noWrap
                sx={{
                  color:
                    option.kind === 'all' || option.kind === 'none' ? 'text.secondary' : undefined,
                }}
              >
                {option.primary}
              </Typography>
              {option.secondary && (
                <Typography
                  variant="caption"
                  noWrap
                  sx={{ color: 'text.secondary', display: 'block' }}
                >
                  {option.secondary}
                </Typography>
              )}
            </Box>
          </li>
        );
      }}
      renderInput={(params) => (
        <TextField
          {...params}
          placeholder={t('globalSearch.placeholder')}
          variant="outlined"
          sx={{
            width: { xs: 150, sm: 200, md: 300 },
            mr: 2,
            '& .MuiOutlinedInput-root': {
              backgroundColor: 'rgba(255, 255, 255, 0.15)',
              '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.25)' },
              '& fieldset': { borderColor: 'rgba(255, 255, 255, 0.3)' },
              '&:hover fieldset': { borderColor: 'rgba(255, 255, 255, 0.5)' },
              // #186: rgba(255,255,255,0.7) on the AppBar computes to 5.04:1.
              '&.Mui-focused fieldset': { borderColor: 'rgba(255, 255, 255, 0.7)' },
            },
            '& .MuiInputBase-input': { color: 'white' },
            '& .MuiInputBase-input::placeholder': {
              color: 'rgba(255, 255, 255, 0.7)',
              opacity: 1,
            },
          }}
          slotProps={{
            ...params.slotProps,
            input: {
              ...params.slotProps.input,
              startAdornment: (
                <InputAdornment position="start">
                  <IconButton
                    size="small"
                    onClick={submit}
                    aria-label={t('app.submitSearch')}
                    sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.5 }}
                  >
                    <SearchIcon />
                  </IconButton>
                </InputAdornment>
              ),
              endAdornment: query ? (
                <InputAdornment position="end">
                  <IconButton
                    size="small"
                    onClick={reset}
                    aria-label={t('contacts.searchClear')}
                    sx={{ color: 'rgba(255, 255, 255, 0.7)', p: 0.5 }}
                  >
                    <ClearIcon fontSize="small" />
                  </IconButton>
                </InputAdornment>
              ) : null,
            },
          }}
        />
      )}
    />
  );
}
