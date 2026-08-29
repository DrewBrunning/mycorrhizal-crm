import {
  Box,
  Button,
  Checkbox,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  ListItemText,
  MenuItem,
  Select,
  Typography,
} from '@mui/material';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import type { Activity } from '../api/activities';
import type { Note } from '../api/notes';
import {
  TIMELINE_BUCKETS,
  TIMELINE_TYPES,
  type TimelineBucket,
  type TimelineItem,
  type TimelineType,
} from '../api/timeline';
import { useTimeline } from '../hooks/useTimeline';
import AppDialog from './AppDialog';
import ContactTimeline from './ContactTimeline';

interface TimelineExplorerDialogProps {
  open: boolean;
  onClose: () => void;
  contactId: number | undefined;
  // The same handlers the page's preview list uses, so editing a note/activity
  // or deleting a completion from inside the explorer behaves identically to
  // doing it on the section itself.
  onEditItem: (type: 'note' | 'activity', item: Note | Activity) => void;
  onDeleteCompletion?: (completionId: number) => void;
  // Bumped by the page whenever its timeline data changes (a note/activity
  // edit or a completion delete that happened through the page-level dialogs
  // the explorer delegates to), so the explorer's own paginated fetch stays
  // in sync instead of showing stale rows.
  revision: number;
}

/**
 * T78 timeline explorer: the full, paginated version of the contact's
 * merged timeline, filterable by event type (multi-select) and recency
 * bucket, driven by the T66 cursor endpoint. "View all" must not become a
 * second unbounded fetch -- the list pages via next_cursor ("Load more").
 */
export default function TimelineExplorerDialog({
  open,
  onClose,
  contactId,
  onEditItem,
  onDeleteCompletion,
  revision,
}: TimelineExplorerDialogProps) {
  const { t } = useTranslation();
  const {
    items,
    nextCursor,
    loading,
    loadingMore,
    error,
    types,
    setTypes,
    bucket,
    setBucket,
    refresh,
    loadMore,
  } = useTimeline(contactId);

  // Fetch a fresh first page whenever the dialog opens, the filters change
  // (refresh is memoized on types/bucket), or the page's timeline data
  // changes underneath us (revision). No fetch while closed.
  useEffect(() => {
    if (open) refresh();
  }, [open, refresh, revision]);

  const timelineItems: Array<{ type: TimelineType; data: TimelineItem['data']; date: string }> =
    items.map((it) => ({ type: it.type, data: it.data, date: it.date }));

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{t('timeline.explorerTitle')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
            <FormControl size="small" sx={{ minWidth: 260, flex: 1 }}>
              <InputLabel id="timeline-type-filter-label">{t('timeline.filterType')}</InputLabel>
              <Select
                labelId="timeline-type-filter-label"
                multiple
                value={types}
                onChange={(e) => setTypes(e.target.value as TimelineType[])}
                label={t('timeline.filterType')}
                renderValue={(selected) => {
                  // Empty selection means "all" (the backend treats an absent
                  // ?type= the same way), so show "All types" for both empty
                  // and fully-selected rather than a bare comma-joined list.
                  if (selected.length === 0 || selected.length === TIMELINE_TYPES.length) {
                    return t('timeline.allTypes');
                  }
                  return selected.map((tt) => t(`timeline.types.${tt}`)).join(', ');
                }}
              >
                {TIMELINE_TYPES.map((tt) => (
                  <MenuItem key={tt} value={tt}>
                    {/* The MenuItem itself is `role="option"` and already
                        carries aria-selected -- that's the accessible
                        selection state. A second, separately-focusable
                        <input type="checkbox"> inside it is both a nested
                        interactive control (axe nested-interactive) and, on
                        its own, an unlabeled form field (axe label) -- issue
                        #259. Removing it from the tab order and the a11y
                        tree via tabIndex={-1}/aria-hidden leaves it as the
                        purely visual checkmark it was always meant to be. */}
                    <Checkbox
                      checked={types.includes(tt)}
                      size="small"
                      tabIndex={-1}
                      disableRipple
                      slotProps={{
                        input: { 'aria-hidden': true },
                      }}
                    />
                    <ListItemText primary={t(`timeline.types.${tt}`)} />
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <FormControl size="small" sx={{ minWidth: 180 }}>
              <InputLabel id="timeline-bucket-filter-label">
                {t('timeline.filterBucket')}
              </InputLabel>
              <Select
                labelId="timeline-bucket-filter-label"
                value={bucket}
                onChange={(e) => setBucket(e.target.value as TimelineBucket)}
                label={t('timeline.filterBucket')}
              >
                {TIMELINE_BUCKETS.map((b) => (
                  <MenuItem key={b} value={b}>
                    {t(`timeline.buckets.${b}`)}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Box>

          {error && (
            <Typography
              variant="body2"
              sx={{
                color: 'error.main',
              }}
            >
              {error}
            </Typography>
          )}

          <Box sx={{ maxHeight: '60vh', overflowY: 'auto' }}>
            {loading ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                <CircularProgress size={28} />
              </Box>
            ) : (
              <ContactTimeline
                timelineItems={timelineItems}
                onEditItem={onEditItem}
                onDeleteCompletion={onDeleteCompletion}
                emptyText={t('timeline.empty')}
              />
            )}
          </Box>

          {nextCursor && (
            <Box sx={{ display: 'flex', justifyContent: 'center' }}>
              {/* Disabled during a refresh too: the cursor belongs to the page
                  it was returned with, so paging on a stale cursor mid-filter-
                  change would fetch the wrong rows. */}
              <Button variant="outlined" onClick={loadMore} disabled={loadingMore || loading}>
                {t('common.loadMore')}
              </Button>
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.close')}</Button>
      </DialogActions>
    </AppDialog>
  );
}
