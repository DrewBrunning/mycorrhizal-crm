import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
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

  const [depth, setDepth] = useState(3);
  // relation is the live input value (no request per keystroke); appliedRelation
  // is the last one actually sent, so a depth change keeps the applied filter
  // instead of silently dropping it while the input still shows it.
  const [relation, setRelation] = useState('');
  const [appliedRelation, setAppliedRelation] = useState('');

  // Reload whenever the anchor or depth changes; the relation filter applies
  // on submit.
  useEffect(() => {
    refresh({ depth, relation: appliedRelation.trim() || undefined, overrideUid: contactUid });
  }, [contactUid, depth, refresh, appliedRelation]);

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
              {idx > 0 && <ArrowForwardIcon sx={{ fontSize: 14, color: 'text.disabled' }} />}
              <Link
                component="button"
                variant="body2"
                underline="hover"
                onClick={() => navigate(`/contacts/${step.contact_id}`)}
                sx={{ color: 'text.secondary' }}
              >
                {step.contact_name}
              </Link>
              <Typography variant="caption" color="text.disabled">
                ({t(`relationships.types.${step.relation}`, step.relation)})
              </Typography>
            </Box>
          ))}
        </Box>
      </Stack>
    </Paper>
  );

  return (
    <Stack spacing={1.5}>
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
            {connections.chains.map(renderChain)}
          </Stack>
        </>
      )}
    </Stack>
  );
}
