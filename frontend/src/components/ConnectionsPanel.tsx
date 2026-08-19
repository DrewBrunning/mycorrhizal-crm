import { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router';
import {
  Box,
  Typography,
  Stack,
  Paper,
  TextField,
  MenuItem,
  Button,
  CircularProgress,
  Alert,
  Chip,
  Link,
  Divider,
} from '@mui/material';
import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import { useTranslation } from 'react-i18next';
import { useConnections } from '../hooks/useConnections';
import { GraphChain } from '../api/graph';

interface ConnectionsPanelProps {
  contactUid: string;
}

const DEPTHS = [1, 2, 3, 4, 5];

// T114: how many chains the panel previews before the "View all" toggle.
const PREVIEW_LIMIT = 5;

// ConnectionsPanel is the contact page's multi-hop traversal surface (T10):
// "who is this person connected to, and how?" Given the contact, it lists every
// reachable person within a chosen depth, each with its relation chain
// ("John's sister's husband" renders as Sister (sister_of) → Husband
// (spouse_of)). The optional relation box accepts a canonical token or a
// registry synonym ("brother" → sibling_of, T11's synonym consumer).
export default function ConnectionsPanel({ contactUid }: ConnectionsPanelProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { connections, loading, error, refresh } = useConnections(contactUid);

  // Default to 1 hop, not 3: a 3-hop traversal floods a contact page's
  // Connections card with second/third-degree strangers before the user has
  // asked for them (testing feedback).
  const [depth, setDepth] = useState(1);
  // relation is the live input value (no request per keystroke); appliedRelation
  // is the last one actually sent, so a depth change keeps the applied filter
  // instead of silently dropping it while the input still shows it.
  const [relation, setRelation] = useState('');
  const [appliedRelation, setAppliedRelation] = useState('');
  // T114: the panel previews the first 5 chains and reveals the rest on
  // demand (the timeline's T78 pattern) -- a deep network can otherwise
  // render dozens of cards here. Re-collapses whenever a new result set
  // lands (depth/relation change), so a fresh query never re-opens a long
  // list the user didn't ask to expand.
  const [showAll, setShowAll] = useState(false);

  // T31 made this panel render on every contact page view (it used to mount
  // only when its tab was opened), so the depth-3 traversal must not fire as
  // part of the page's critical path. Defer the initial fetch until the panel
  // is about to scroll into view; jsdom has no IntersectionObserver, so
  // component tests fall back to loading immediately.
  const panelRef = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const el = panelRef.current;
    if (!el) return;
    if (typeof IntersectionObserver === 'undefined') {
      setVisible(true);
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setVisible(true);
          observer.disconnect();
        }
      },
      { rootMargin: '300px 0px' }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  // Reload whenever the anchor or depth changes; the relation filter applies
  // on submit. Skipped until the panel has first become visible.
  useEffect(() => {
    if (!visible) return;
    refresh({ depth, relation: appliedRelation.trim() || undefined, overrideUid: contactUid });
  }, [contactUid, depth, refresh, appliedRelation, visible]);

  // T114: a new result set re-collapses the preview.
  useEffect(() => {
    setShowAll(false);
  }, [connections]);

  const handleApplyRelation = () => {
    setAppliedRelation(relation.trim());
    refresh({ depth, relation: relation.trim() || undefined, overrideUid: contactUid });
  };

  const renderChain = (chain: GraphChain) => (
    <Paper key={chain.target_vcard_uid} variant="outlined" sx={{ p: 1.5 }}>
      <Stack spacing={0.5}>
        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0.5 }}>
          <Typography variant="body2" sx={{ fontWeight: 600, overflowWrap: 'anywhere' }}>
            {chain.target_name}
          </Typography>
          <Chip size="small" label={t('connections.depthLabel', { depth: chain.depth })} />
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0.5 }}>
          {chain.steps.map((step, idx) => (
            <Box key={step.contact_vcard_uid} sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
              {/* #187: both were text.disabled (2.62:1 on parchment) -- that
                  token is MUI's lowest-contrast text and reads as "disabled/
                  greyed out". The direction arrow and relation label are not
                  disabled, they're the content that gives the chain its
                  meaning, so they get text.secondary (6.35:1) instead. Swept
                  the rest of the codebase's text.disabled uses too -- the
                  other sites (ContactInformation.tsx, CustomFieldValueRow.tsx)
                  render an empty-value placeholder ('-'), which is the
                  token's actual intended use, so those are left alone. */}
              {idx > 0 && <ArrowForwardIcon sx={{ fontSize: 14, color: 'text.secondary' }} />}
              <Link
                component="button"
                variant="body2"
                underline="hover"
                onClick={() => navigate(`/contacts/${step.contact_id}`)}
                sx={{ color: 'text.secondary' }}
              >
                {step.contact_name}
              </Link>
              <Typography variant="caption" color="text.secondary">
                ({t(`relationships.types.${step.relation}`, step.relation)})
              </Typography>
            </Box>
          ))}
        </Box>
      </Stack>
    </Paper>
  );

  return (
    <Stack spacing={1.5} ref={panelRef}>
      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap' }}>
        <TextField
          select
          size="small"
          label={t('connections.depth')}
          value={depth}
          onChange={(e) => setDepth(Number(e.target.value))}
          sx={{ minWidth: 110 }}
        >
          {DEPTHS.map((d) => (
            <MenuItem key={d} value={d}>
              {d}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label={t('connections.relationFilter')}
          placeholder={t('connections.relationPlaceholder')}
          value={relation}
          onChange={(e) => setRelation(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleApplyRelation();
          }}
          sx={{ minWidth: 180 }}
        />
        <Button size="small" variant="outlined" onClick={handleApplyRelation}>
          {t('connections.apply')}
        </Button>
      </Box>

      {error && <Alert severity="error" sx={{ py: 0 }}>{error}</Alert>}

      {loading ? (
        <CircularProgress size={24} />
      ) : !connections ? null : connections.chains.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
          {appliedRelation.trim()
            ? t('connections.noRelationMatches', { relation: appliedRelation.trim() })
            : t('connections.empty')}
        </Typography>
      ) : (
        <>
          {connections.chains.length > 0 && (
            <>
              <Divider />
              <Typography variant="subtitle2" color="text.secondary">
                {t('connections.results', { count: connections.chains.length })}
              </Typography>
            </>
          )}
          <Stack spacing={1}>
            {(showAll ? connections.chains : connections.chains.slice(0, PREVIEW_LIMIT)).map(renderChain)}
          </Stack>
          {connections.chains.length > PREVIEW_LIMIT && (
            <Button size="small" variant="outlined" onClick={() => setShowAll((v) => !v)}>
              {showAll ? t('connections.showLess') : t('connections.viewAll')}
            </Button>
          )}
        </>
      )}
    </Stack>
  );
}
