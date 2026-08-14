import { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Avatar,
  Typography,
  Chip,
  Divider,
  Alert,
  CircularProgress,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import AppDialog from './AppDialog';
import MergeContactsDialog from './MergeContactsDialog';
import { useDuplicatePairs } from '../hooks/useDuplicatePairs';
import { DuplicatePair, DuplicateReason } from '../api/duplicates';
import { Contact } from '../api/contacts';
import { useSnackbar } from '../context/SnackbarContext';

interface ReviewDuplicatesDialogProps {
  open: boolean;
  onClose: () => void;
}

function contactName(c: Contact): string {
  return [c.firstname, c.lastname].filter(Boolean).join(' ').trim() || c.uid || '';
}

// PairLine renders one candidate pair with its reasons, confidence, and the
// Merge / Not-a-duplicate actions (T93 — docs/fork-plan/tickets/
// 137-T93-duplicate-scan-endpoint-and-review.md).
function PairLine({
  pair,
  onMerge,
  onDismiss,
}: {
  pair: DuplicatePair;
  onMerge: (pair: DuplicatePair) => void;
  onDismiss: (pair: DuplicatePair) => void;
}) {
  const { t } = useTranslation();
  const [dismissing, setDismissing] = useState(false);

  const handleDismiss = async () => {
    // Dismissal is permanent (no undo surface exists) — require a real
    // confirmation, matching the bulk-delete pattern.
    if (!window.confirm(t('duplicates.dismissConfirm'))) return;
    setDismissing(true);
    try {
      await onDismiss(pair);
    } finally {
      setDismissing(false);
    }
  };

  const reasonLabel = (r: DuplicateReason) => {
    switch (r) {
      case 'email': return t('duplicates.reason.email');
      case 'name': return t('duplicates.reason.name');
      case 'phone': return t('duplicates.reason.phone');
      default: return r;
    }
  };

  const renderContact = (c: Contact) => (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0, flex: 1 }}>
      <Avatar src={c.photo_thumbnail || undefined} sx={{ width: 40, height: 40, bgcolor: 'primary.main', flexShrink: 0 }}>
        {c.firstname.charAt(0)}
      </Avatar>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>
          {contactName(c)}
        </Typography>
        <Typography variant="caption" color="text.secondary" noWrap>
          {c.email || c.phone || (c.archived ? t('contacts.archived') : '')}
        </Typography>
        {c.archived && (
          <Chip label={t('contacts.archived')} size="small" sx={{ height: 16, fontSize: '0.65rem', mt: 0.25 }} />
        )}
      </Box>
    </Box>
  );

  return (
    <Box sx={{ py: 1.5 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
        {renderContact(pair.a)}
        <Typography variant="body2" color="text.secondary" sx={{ px: 0.5 }}>
          {t('duplicates.vs')}
        </Typography>
        {renderContact(pair.b)}
      </Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.75, flexWrap: 'wrap' }}>
        {pair.reasons.map((r) => (
          <Chip key={r} label={reasonLabel(r)} size="small" color="default" sx={{ height: 20, fontSize: '0.7rem' }} />
        ))}
        <Chip
          label={t('duplicates.confidence', { percent: Math.round(pair.confidence * 100) })}
          size="small"
          color={pair.confidence >= 0.9 ? 'primary' : 'default'}
          sx={{ height: 20, fontSize: '0.7rem' }}
        />
        <Box sx={{ flexGrow: 1 }} />
        <Button size="small" variant="outlined" onClick={() => onMerge(pair)}>
          {t('contactMerge.mergeButton')}
        </Button>
        <Button size="small" color="inherit" onClick={handleDismiss} disabled={dismissing}>
          {dismissing ? <CircularProgress size={14} /> : t('duplicates.notDuplicate')}
        </Button>
      </Box>
    </Box>
  );
}

// ReviewDuplicatesDialog is T93's web review surface: a "Review duplicates"
// dialog reachable from the Contacts page, listing the scan's candidate pairs
// strongest-first. Each pair offers Merge (opening MergeContactsDialog
// pre-populated with both contacts) and a persistent Not-a-duplicate
// dismissal.
export default function ReviewDuplicatesDialog({ open, onClose }: ReviewDuplicatesDialogProps) {
  const { t } = useTranslation();
  const { showError } = useSnackbar();
  const { pairs, total, loading, error, refresh, dismiss } = useDuplicatePairs({ showError });
  const [mergePair, setMergePair] = useState<{ a: Contact; b: Contact } | null>(null);

  useEffect(() => {
    if (open) refresh();
  }, [open, refresh]);

  const handleDismiss = async (pair: DuplicatePair) => {
    try {
      await dismiss(pair);
    } catch {
      // useDuplicatePairs surfaced the error via the snackbar.
    }
  };

  const handleMerged = async () => {
    setMergePair(null);
    // The loser is gone and the survivor may now duplicate someone else —
    // re-run the whole scan so the list reflects reality.
    await refresh();
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{t('duplicates.reviewTitle')}</DialogTitle>
      <DialogContent>
        {loading && pairs.length === 0 ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress />
          </Box>
        ) : error ? (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        ) : pairs.length === 0 ? (
          <Alert severity="success">{t('duplicates.none')}</Alert>
        ) : (
          <>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {t('duplicates.count', { count: total })}
            </Typography>
            {pairs.map((pair, index) => (
              <Box key={`${pair.a.uid}-${pair.b.uid}`}>
                {index > 0 && <Divider />}
                <PairLine pair={pair} onMerge={setMergePair} onDismiss={handleDismiss} />
              </Box>
            ))}
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.close')}</Button>
      </DialogActions>

      {mergePair && (
        <MergeContactsDialog
          open
          onClose={() => setMergePair(null)}
          onMerged={handleMerged}
          pair={mergePair}
        />
      )}
    </AppDialog>
  );
}
