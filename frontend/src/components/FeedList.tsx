import AutorenewIcon from '@mui/icons-material/Autorenew';
import BlockIcon from '@mui/icons-material/Block';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Feed } from '../api/feeds';

interface FeedListProps {
  feeds: Feed[];
  /** Contact.VCardUID -> display name, for `contact` feeds. */
  contactNames: Map<string, string>;
  onRotate: (feed: Feed) => Promise<void>;
  onRevoke: (feed: Feed) => Promise<void>;
  onRevokeAll: () => Promise<void>;
}

type Pending = { action: 'rotate' | 'revoke'; feed: Feed } | { action: 'revokeAll' } | null;

// Rotate, Revoke and Revoke all each ask before acting: all three end (or
// replace) a URL a reader may be subscribed to, and none can be undone.
export default function FeedList({
  feeds,
  contactNames,
  onRotate,
  onRevoke,
  onRevokeAll,
}: FeedListProps) {
  const { t } = useTranslation();
  const [pending, setPending] = useState<Pending>(null);
  const [busy, setBusy] = useState(false);

  const kindLabel = (feed: Feed) =>
    feed.kind === 'contact'
      ? t('feeds.kindContact', {
          name: contactNames.get(feed.entity_id) || t('feeds.unknownContact'),
        })
      : t('feeds.kind.aggregate');

  const formatDate = (value: string | null) =>
    value ? new Date(value).toLocaleString() : t('feeds.never');

  const confirm = async () => {
    if (!pending) return;
    setBusy(true);
    try {
      if (pending.action === 'revokeAll') await onRevokeAll();
      else if (pending.action === 'revoke') await onRevoke(pending.feed);
      else await onRotate(pending.feed);
      setPending(null);
    } catch {
      // The owner surfaced the error; keep the dialog open so it can be retried.
    } finally {
      setBusy(false);
    }
  };

  const dialogName = pending && pending.action !== 'revokeAll' ? pending.feed.name : '';

  return (
    <>
      <Stack direction="row" sx={{ justifyContent: 'flex-end', mb: 1 }}>
        <Button
          variant="outlined"
          size="small"
          color="error"
          startIcon={<BlockIcon />}
          onClick={() => setPending({ action: 'revokeAll' })}
          disabled={feeds.length === 0}
        >
          {t('feeds.revokeAllButton')}
        </Button>
      </Stack>
      <TableContainer
        component={Paper}
        variant="outlined"
        tabIndex={0}
        role="region"
        aria-label={t('feeds.title')}
      >
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>{t('feeds.columns.name')}</TableCell>
              <TableCell>{t('feeds.columns.kind')}</TableCell>
              <TableCell>{t('feeds.columns.detail')}</TableCell>
              <TableCell>{t('feeds.columns.created')}</TableCell>
              <TableCell>{t('feeds.columns.lastAccessed')}</TableCell>
              <TableCell>{t('feeds.columns.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {feeds.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} align="center">
                  <Typography sx={{ color: 'text.secondary' }}>{t('feeds.noFeeds')}</Typography>
                </TableCell>
              </TableRow>
            ) : (
              feeds.map((feed) => (
                <TableRow key={feed.id}>
                  <TableCell>{feed.name}</TableCell>
                  <TableCell>{kindLabel(feed)}</TableCell>
                  <TableCell>{t(`feeds.detail.${feed.detail}`)}</TableCell>
                  <TableCell>{formatDate(feed.created_at)}</TableCell>
                  <TableCell>{formatDate(feed.last_accessed_at)}</TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={0.5}>
                      <Tooltip title={t('feeds.rotateDialog.title')}>
                        <IconButton
                          size="small"
                          onClick={() => setPending({ action: 'rotate', feed })}
                          aria-label={t('feeds.rotateAria', { name: feed.name })}
                        >
                          <AutorenewIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title={t('feeds.revokeDialog.title')}>
                        <IconButton
                          size="small"
                          color="error"
                          onClick={() => setPending({ action: 'revoke', feed })}
                          aria-label={t('feeds.revokeAria', { name: feed.name })}
                        >
                          <BlockIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    </Stack>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={pending !== null} onClose={() => !busy && setPending(null)}>
        <DialogTitle>
          {pending?.action === 'revokeAll'
            ? t('feeds.revokeAllDialog.title')
            : pending?.action === 'rotate'
              ? t('feeds.rotateDialog.title')
              : t('feeds.revokeDialog.title')}
        </DialogTitle>
        <DialogContent>
          <Typography>
            {pending?.action === 'revokeAll'
              ? t('feeds.revokeAllDialog.message', { count: feeds.length })
              : pending?.action === 'rotate'
                ? t('feeds.rotateDialog.message', { name: dialogName })
                : t('feeds.revokeDialog.message', { name: dialogName })}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPending(null)} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="contained"
            color={pending?.action === 'rotate' ? 'primary' : 'error'}
            onClick={() => void confirm()}
            disabled={busy}
          >
            {pending?.action === 'revokeAll'
              ? t('feeds.revokeAllDialog.confirm')
              : pending?.action === 'rotate'
                ? t('feeds.rotateDialog.confirm')
                : t('feeds.revokeDialog.confirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
