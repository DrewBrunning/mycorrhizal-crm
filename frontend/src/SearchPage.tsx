import { useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Box,
  Typography,
  TextField,
  InputAdornment,
  IconButton,
  CircularProgress,
  Alert,
  Stack,
  Card,
  CardContent,
  Divider,
  Link,
  Chip,
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import ClearIcon from '@mui/icons-material/Clear';
import { useSearch } from './hooks/useSearch';
import { useDateFormat } from './DateFormatProvider';

// SearchPage is the global full-text search surface (T11): one box that
// searches contacts, notes, and interactions together. Results are grouped
// into three sections so a note/activity match is visible — the whole point
// of the ticket ("frontend search surfaces notes and interactions, not just
// contacts").
export default function SearchPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { formatDate } = useDateFormat();
  const [searchParams, setSearchParams] = useSearchParams();
  const { result, loading, error, runSearch } = useSearch();

  const qParam = searchParams.get('q') || '';
  const [input, setInput] = useState(qParam);
  // Tracks the q value this component itself last wrote to the URL, so the
  // sync effect below can tell "the URL changed because we wrote it"
  // (ignore) apart from "the URL changed because something else navigated
  // here" — e.g. the top-nav search bar submitting a new query while this
  // page is already mounted. React Router doesn't remount SearchPage for a
  // same-route param change, so without this, `input` (seeded from the URL
  // only at first mount) never picks up the new query and the debounce
  // effect below silently writes the stale one back over the URL.
  const ownWriteRef = useRef(qParam);

  useEffect(() => {
    if (qParam !== ownWriteRef.current) {
      setInput(qParam);
    }
  }, [qParam]);

  useEffect(() => {
    const timer = setTimeout(() => {
      runSearch(input);
      // Keep the URL in sync so the query is shareable/reloadable.
      const nextQ = input.trim().length >= 2 ? input.trim() : '';
      ownWriteRef.current = nextQ;
      setSearchParams(nextQ ? { q: nextQ } : {}, { replace: true });
    }, 300);
    return () => clearTimeout(timer);
  }, [input, runSearch, setSearchParams]);

  const openContact = (id: number) => navigate(`/contacts/${id}`);

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" gutterBottom sx={{ mb: 2 }}>
        {t('search.title')}
      </Typography>

      <TextField
        autoFocus
        label={t('search.placeholder')}
        value={input}
        onChange={(e) => setInput(e.target.value)}
        fullWidth
        size="small"
        slotProps={{
          input: {
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon />
              </InputAdornment>
            ),
            endAdornment: input ? (
              <InputAdornment position="end">
                <IconButton size="small" onClick={() => setInput('')} aria-label={t('search.clear')}>
                  <ClearIcon fontSize="small" />
                </IconButton>
              </InputAdornment>
            ) : undefined,
          },
        }}
      />

      {loading && <CircularProgress size={24} sx={{ mt: 2 }} />}
      {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}

      {!loading && !error && result && input.trim().length >= 2 && (
        <Stack spacing={2} sx={{ mt: 2 }}>
          {result.resolved_relation && (
            <Alert severity="info" sx={{ py: 0 }}>
              {t('search.resolvedRelation', { relation: t(`relationships.types.${result.resolved_relation}`, result.resolved_relation) })}
            </Alert>
          )}

          {result.contacts.length === 0 && result.notes.length === 0 && result.activities.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              {t('search.noResults', { query: result.query })}
            </Typography>
          )}

          {result.contacts.length > 0 && (
            <Card>
              <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
                <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                  {t('search.contacts', { count: result.contacts.length })}
                </Typography>
                <Stack spacing={0.5}>
                  {result.contacts.map((c) => (
                    <Box key={c.uid} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Link
                        component="button"
                        variant="body1"
                        underline="hover"
                        onClick={() => openContact(c.id)}
                        sx={{ textAlign: 'left' }}
                      >
                        {c.firstname} {c.lastname}
                      </Link>
                      {c.primary_email && (
                        <Typography variant="caption" color="text.secondary">{c.primary_email}</Typography>
                      )}
                    </Box>
                  ))}
                </Stack>
              </CardContent>
            </Card>
          )}

          {result.notes.length > 0 && (
            <Card>
              <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
                <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                  {t('search.notes', { count: result.notes.length })}
                </Typography>
                <Stack spacing={1}>
                  {result.notes.map((n) => (
                    <Box key={n.id}>
                      <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                        {n.content}
                      </Typography>
                      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap', mt: 0.5 }}>
                        {n.contact_name ? (
                          <Chip size="small" label={n.contact_name} onClick={() => n.contact_id && openContact(n.contact_id)} />
                        ) : (
                          <Chip size="small" label={t('search.unfiled')} />
                        )}
                        <Typography variant="caption" color="text.secondary">
                          {formatDate(n.date)}
                        </Typography>
                      </Box>
                    </Box>
                  ))}
                </Stack>
              </CardContent>
            </Card>
          )}

          {result.activities.length > 0 && (
            <Card>
              <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
                <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                  {t('search.activities', { count: result.activities.length })}
                </Typography>
                <Stack spacing={0.5}>
                  {result.activities.map((a) => (
                    <Box key={a.id}>
                      <Typography variant="body1">{a.title}</Typography>
                      <Typography variant="caption" color="text.secondary">
                        {formatDate(a.date)}
                      </Typography>
                    </Box>
                  ))}
                </Stack>
              </CardContent>
            </Card>
          )}
        </Stack>
      )}

      {!loading && !error && input.trim().length < 2 && (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
          {t('search.minLengthHint')}
        </Typography>
      )}
      <Divider sx={{ my: 2 }} />
    </Box>
  );
}
