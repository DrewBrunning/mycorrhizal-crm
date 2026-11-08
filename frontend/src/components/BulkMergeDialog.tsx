import {
  Button,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Radio,
  RadioGroup,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Contact } from '../api/contacts';
import AppDialog from './AppDialog';
import MergeContactsDialog from './MergeContactsDialog';

interface BulkMergeDialogProps {
  open: boolean;
  // The selected contacts (2 or more), resolved from the loaded page.
  contacts: Contact[];
  // Called exactly once when the flow ends -- completed, or cancelled -- with
  // how many merges were committed, so the page can refetch only when needed.
  onFinish: (mergedCount: number) => void | Promise<void>;
}

function contactName(c: Contact): string {
  return [c.firstname, c.lastname].filter(Boolean).join(' ').trim() || c.uid || '';
}

// #2197: merge three or more selected contacts. The backend merge is
// pairwise, so this picks a primary and then walks the others through the
// existing preview -> resolve -> commit dialog one at a time (each step
// reviews its own field conflicts). The survivor of each step -- which the
// user may swap in the pair dialog -- becomes the primary of the next, so
// nothing the user decided is overridden. A cancel part-way keeps the merges
// already committed; they are real and are reported through onFinish.
export default function BulkMergeDialog({ open, contacts, onFinish }: BulkMergeDialogProps) {
  const { t } = useTranslation();
  const [primaryId, setPrimaryId] = useState<number>(contacts[0].ID);
  const [started, setStarted] = useState(false);
  const [mergedIds, setMergedIds] = useState<number[]>([]);
  const finished = useRef(false);
  const cancelTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(cancelTimer.current), []);

  const primary = contacts.find((c) => c.ID === primaryId) ?? contacts[0];
  const remaining = contacts.filter((c) => c.ID !== primary.ID && !mergedIds.includes(c.ID));
  const total = contacts.length - 1;
  const next = remaining[0];
  // Stable identity: MergeContactsDialog resets its keeper whenever `pair`
  // changes, so a fresh object per render would undo the user's swap.
  const pair = useMemo(() => (next ? { a: primary, b: next } : null), [primary, next]);

  const finish = (count: number) => {
    if (finished.current) return;
    finished.current = true;
    void onFinish(count);
  };

  if (!started) {
    return (
      <AppDialog open={open} onClose={() => finish(0)} maxWidth="sm" fullWidth>
        <DialogTitle>{t('bulk.mergeTitle')}</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ color: 'text.secondary', mb: 1 }}>
            {t('bulk.mergeChoosePrimary')}
          </Typography>
          <RadioGroup
            value={String(primary.ID)}
            onChange={(e) => setPrimaryId(Number(e.target.value))}
            aria-label={t('bulk.mergePrimaryLabel')}
          >
            {contacts.map((c) => (
              <FormControlLabel
                key={c.ID}
                value={String(c.ID)}
                control={<Radio size="small" />}
                label={contactName(c)}
              />
            ))}
          </RadioGroup>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => finish(0)}>{t('common.cancel')}</Button>
          <Button variant="contained" onClick={() => setStarted(true)}>
            {t('bulk.mergeContinue')}
          </Button>
        </DialogActions>
      </AppDialog>
    );
  }

  if (!pair || !next) return null;

  return (
    <MergeContactsDialog
      open={open}
      pair={pair}
      progressLabel={t('bulk.mergeProgress', {
        current: mergedIds.length + 1,
        total,
      })}
      // The pair dialog calls onClose both for Cancel and right before
      // onMerged after a successful commit. Defer the cancel one tick so a
      // commit (which clears the timer in onMerged) is not mistaken for it.
      onClose={() => {
        cancelTimer.current = setTimeout(() => finish(mergedIds.length), 0);
      }}
      onMerged={(keeperId) => {
        clearTimeout(cancelTimer.current);
        const survivor = keeperId === next.ID ? next : primary;
        const loser = survivor === next ? primary : next;
        const merged = [...mergedIds, loser.ID];
        setPrimaryId(survivor.ID);
        setMergedIds(merged);
        // The primary may have been the one folded away (a swap); it is
        // excluded below by uid, and the survivor leads the next pair.
        if (merged.length >= total) finish(merged.length);
      }}
    />
  );
}
