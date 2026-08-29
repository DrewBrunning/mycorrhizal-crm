import CheckIcon from '@mui/icons-material/Check';
import CloseIcon from '@mui/icons-material/Close';
import {
  Alert,
  Box,
  Button,
  Chip,
  Divider,
  IconButton,
  Paper,
  Stack,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type Contact, getContactsByUid } from '../api/contacts';
import {
  acceptRelationshipEdge,
  deleteRelationshipEdge,
  listSuggestedRelationshipEdges,
  type RelationshipEdge,
} from '../api/relationshipEdges';
import { handleFetchError } from '../utils/errorHandler';

// T104: the global relationship-suggestion review surface. The per-contact
// RelationshipEdgeList only shows one contact's edges; this one pages through
// every pending suggested edge across the whole graph (the backend list
// endpoint supports ?status=suggested without a contact_id), so the "Suggest
// relationships" button on the Data page has an inbox to point at. Reloads
// whenever `loadKey` changes (the Data page bumps it after each suggest run).
interface RelationshipSuggestionsInboxProps {
  loadKey: number;
}

export default function RelationshipSuggestionsInbox({
  loadKey,
}: RelationshipSuggestionsInboxProps) {
  const { t } = useTranslation();
  const [edges, setEdges] = useState<RelationshipEdge[]>([]);
  const [contactsByUid, setContactsByUid] = useState<Map<string, Contact>>(new Map());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  // Internal reload trigger (accept/reject/refresh bump it); loadKey is the
  // Data page's external trigger after a suggest run.
  const [reloadTick, setReloadTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    listSuggestedRelationshipEdges()
      .then(async (response) => {
        if (cancelled) return;
        const fetched = response.relationship_edges || [];
        setEdges(fetched);
        const uids = new Set<string>();
        for (const e of fetched) {
          uids.add(e.source_id);
          uids.add(e.target_id);
        }
        if (uids.size > 0) {
          setContactsByUid(await getContactsByUid([...uids]));
        } else {
          setContactsByUid(new Map());
        }
        setLoaded(true);
      })
      .catch((err) => {
        if (cancelled) return;
        handleFetchError(err, 'loading relationship suggestions');
        setError(t('settings.data.propose.suggestionsLoadError'));
        setLoaded(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [loadKey, reloadTick, t]);

  const reload = () => setReloadTick((n) => n + 1);

  const handleAccept = async (id: string) => {
    setBusyId(id);
    try {
      await acceptRelationshipEdge(id);
      await reload();
    } catch (err) {
      handleFetchError(err, 'accepting suggested relationship');
    } finally {
      setBusyId(null);
    }
  };

  const handleReject = async (id: string) => {
    setBusyId(id);
    try {
      await deleteRelationshipEdge(id);
      await reload();
    } catch (err) {
      handleFetchError(err, 'rejecting suggested relationship');
    } finally {
      setBusyId(null);
    }
  };

  const nameFor = (uid: string) => {
    const c = contactsByUid.get(uid);
    if (!c) return t('relationships.unknownContact');
    return `${c.firstname} ${c.lastname}`.trim();
  };

  if (!loaded) {
    return null;
  }

  return (
    <Box sx={{ mt: 2 }}>
      <Divider sx={{ mb: 1.5 }} />
      <Typography
        variant="subtitle2"
        component="h3"
        sx={{
          color: 'text.secondary',
          mb: 1,
        }}
      >
        {t('relationships.suggestedRelationships')}
      </Typography>
      {error && (
        <Alert severity="error" sx={{ mb: 1 }}>
          {error}
        </Alert>
      )}
      {loading && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('settings.data.propose.loadingSuggestions')}
        </Alert>
      )}
      {edges.length === 0 && !loading && (
        <Alert severity="info">{t('settings.data.propose.noRelationshipSuggestions')}</Alert>
      )}
      <Stack spacing={1}>
        {edges.map((edge) => {
          const label = t(`relationships.types.${edge.type}`);
          const busy = busyId === edge.id;
          return (
            <Paper key={edge.id} variant="outlined" sx={{ p: 1.5, bgcolor: 'action.hover' }}>
              <Box
                sx={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  gap: 1,
                }}
              >
                <Box sx={{ minWidth: 0 }}>
                  <Typography variant="body2" noWrap>
                    {nameFor(edge.source_id)} · {label} · {nameFor(edge.target_id)}
                  </Typography>
                  <Chip
                    label={t('relationships.suggested')}
                    color="warning"
                    size="small"
                    sx={{ height: 18, mt: 0.5 }}
                  />
                </Box>
                <Box sx={{ display: 'flex', gap: 0.5 }}>
                  <IconButton
                    size="small"
                    color="success"
                    disabled={busy}
                    onClick={() => handleAccept(edge.id)}
                    aria-label={t('relationships.accept')}
                  >
                    <CheckIcon fontSize="small" />
                  </IconButton>
                  <IconButton
                    size="small"
                    color="error"
                    disabled={busy}
                    onClick={() => handleReject(edge.id)}
                    aria-label={t('relationships.reject')}
                  >
                    <CloseIcon fontSize="small" />
                  </IconButton>
                </Box>
              </Box>
            </Paper>
          );
        })}
      </Stack>
      {edges.length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Button size="small" onClick={reload}>
            {t('settings.data.propose.refreshSuggestions')}
          </Button>
        </Box>
      )}
    </Box>
  );
}
