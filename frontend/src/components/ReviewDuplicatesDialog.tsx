import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Typography,
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Contact } from '../api/contacts';
import type { DuplicatePair, DuplicateReason } from '../api/duplicates';
import { useSnackbar } from '../context/SnackbarContext';
import { useDuplicatePairs } from '../hooks/useDuplicatePairs';
import { useFocusAfterRemoval } from '../hooks/useFocusAfterRemoval';
import AppDialog from './AppDialog';
import MergeContactsDialog from './MergeContactsDialog';

interface ReviewDuplicatesDialogProps {
  open: boolean;
  onClose: () => void;
}

const pairKey = (p: DuplicatePair) => `${p.a.uid}-${p.b.uid}`;

function contactName(c: Contact): string {
  return [c.firstname, c.lastname].filter(Boolean).join(' ').trim() || c.uid || '';
}

// PairLine renders one candidate pair with its reasons, confidence, and the
// Merge / Not-a-duplicate actions (T93 — T93).
function PairLine({
  pair,
  onMerge,
  onDismiss,
  dismissRef,
}: {
  pair: DuplicatePair;
  dismissRef?: (el: HTMLButtonElement | null) => void;
  onMerge: (pair: DuplicatePair) => void;
  // The real implementation (ReviewDuplicatesDialog's handleDismiss) is
  // async — it awaits the dismiss API call so the caller can show the
  // spinner and swallow the already-snackbar'd error for its actual
  // duration. A `void`-only signature here was a real type mismatch: it
  // made `await onDismiss(pair)` below type-check as awaiting a
  // non-Promise, which `@typescript-eslint/await-thenable` caught. Widen to
  // `void | Promise<void>` so a synchronous caller still type-checks too.
  onDismiss: (pair: DuplicatePair) => void | Promise<void>;
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
      case 'email':
        return t('duplicates.reason.email');
      case 'name':
        return t('duplicates.reason.name');
      case 'phone':
        return t('duplicates.reason.phone');
      default:
        return r;
    }
  };

  const renderContact = (c: Contact) => (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0, flex: 1 }}>
      <Avatar
        src={c.photo_thumbnail || undefined}
        sx={{ width: 40, height: 40, bgcolor: 'primary.main', flexShrink: 0 }}
      >
        {c.firstname.charAt(0)}
      </Avatar>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body2" noWrap data-user-content sx={{ fontWeight: 500 }}>
          {contactName(c)}
        </Typography>
        <Typography
          variant="caption"
          noWrap
          data-user-content
          sx={{
            color: 'text.secondary',
          }}
        >
          {c.email || c.phone || (c.archived ? t('contacts.archived') : '')}
        </Typography>
        {c.archived && (
          <Chip
            label={t('contacts.archived')}
            size="small"
            sx={{ height: 16, fontSize: '0.65rem', mt: 0.25 }}
          />
        )}
      </Box>
    </Box>
  );

  return (
    <Box sx={{ py: 1.5 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
        {renderContact(pair.a)}
        <Typography
          variant="body2"
          sx={{
            color: 'text.secondary',
            px: 0.5,
          }}
        >
          {t('duplicates.vs')}
        </Typography>
        {renderContact(pair.b)}
      </Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.75, flexWrap: 'wrap' }}>
        {pair.reasons.map((r) => (
          <Chip
            key={r}
            label={reasonLabel(r)}
            size="small"
            color="default"
            sx={{ height: 20, fontSize: '0.7rem' }}
          />
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
        <Button
          size="small"
          color="inherit"
          onClick={() => void handleDismiss()}
          disabled={dismissing}
          ref={dismissRef}
        >
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
    // refresh() catches its own errors internally (sets `error` state);
    // fire-and-forget here is intentional.
    if (open) void refresh();
  }, [open, refresh]);

  // Dismissing unmounts the pair that held focus: move it to a neighbouring
  // pair's button, else the dialog title (#2918, #2920).
  const titleRef = useRef<HTMLHeadingElement>(null);
  const dismissFocus = useFocusAfterRemoval(titleRef, pairs, !loading);

  const handleDismiss = async (pair: DuplicatePair) => {
    dismissFocus.request(pairs.map(pairKey), pairKey(pair));
    try {
      await dismiss(pair);
    } catch {
      // useDuplicatePairs surfaced the error via the snackbar.
      dismissFocus.cancel();
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
      <DialogTitle ref={titleRef} tabIndex={-1}>
        {t('duplicates.reviewTitle')}
      </DialogTitle>
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
            <Typography
              variant="body2"
              sx={{
                color: 'text.secondary',
                mb: 1,
              }}
            >
              {t('duplicates.count', { count: total })}
            </Typography>
            {pairs.map((pair, index) => (
              <Box key={pairKey(pair)}>
                {index > 0 && <Divider />}
                <PairLine
                  pair={pair}
                  onMerge={setMergePair}
                  onDismiss={handleDismiss}
                  dismissRef={dismissFocus.register(pairKey(pair))}
                />
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
