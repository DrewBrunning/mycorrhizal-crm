import {
  Alert,
  Box,
  Button,
  Chip,
  LinearProgress,
  List,
  ListItem,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  acceptShareGrant,
  declineShareGrant,
  deleteShareBlock,
  leaveShareGrant,
  listIncomingShareGrants,
  listShareBlocks,
  type ShareBlock,
  type ShareGrantRecipientView,
} from './api/shareGrants';
import LoadError from './components/LoadError';
import PageHeader from './components/PageHeader';
import ShareConfirmDialog from './components/ShareConfirmDialog';
import SharedGrantContentDialog from './components/SharedGrantContentDialog';
import { useAnnouncer } from './context/AnnouncerContext';
import { useDateFormat } from './DateFormatProvider';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useShareGrantList } from './hooks/useShareGrantList';
import { getErrorMessage } from './utils/errorHandler';

type Confirm = { kind: 'leave' | 'block'; grant: ShareGrantRecipientView };

// "Shared with me": the recipient side of standing shares (ADR 0042 decisions 7,
// 8 and 12; issue #2292). A pending grant shows ONLY the sender, the entity type
// and the per-request label -- no sections, no content -- until it is accepted.
// Blocking is offered on a pending grant only because the API's block is a
// decline with {block: true}; an active grant can be left but not blocked.
export default function SharedWithMePage() {
  const { t } = useTranslation();
  useDocumentTitle(t('standingShares.withMe.title'));
  const { announce } = useAnnouncer();
  const { formatDate } = useDateFormat();
  const { items, nextCursor, loading, error, refresh, loadMore, replace, remove } =
    useShareGrantList<ShareGrantRecipientView>(listIncomingShareGrants);
  const [blocks, setBlocks] = useState<ShareBlock[]>([]);
  const [blocksError, setBlocksError] = useState('');
  const [busyId, setBusyId] = useState<string | null>(null);
  const [actionError, setActionError] = useState('');
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [viewing, setViewing] = useState<ShareGrantRecipientView | null>(null);

  const refreshBlocks = useCallback(async () => {
    try {
      setBlocks(await listShareBlocks());
      setBlocksError('');
    } catch (err) {
      setBlocksError(getErrorMessage(err));
    }
  }, []);

  useEffect(() => {
    void refresh();
    void refreshBlocks();
  }, [refresh, refreshBlocks]);

  const run = async (id: string, action: () => Promise<void>) => {
    setBusyId(id);
    setActionError('');
    try {
      await action();
    } catch (err) {
      setActionError(t('standingShares.withMe.actionFailed', { message: getErrorMessage(err) }));
    } finally {
      setBusyId(null);
    }
  };

  const params = (g: ShareGrantRecipientView) => ({ label: g.label, username: g.from_username });

  const handleAccept = (g: ShareGrantRecipientView) =>
    run(g.id, async () => {
      replace(await acceptShareGrant(g.id));
      announce(t('standingShares.withMe.accepted', params(g)));
    });

  const handleDecline = (g: ShareGrantRecipientView) =>
    run(g.id, async () => {
      await declineShareGrant(g.id, { block: false });
      remove(g.id);
      announce(t('standingShares.withMe.declined', params(g)));
    });

  const handleConfirm = async () => {
    if (!confirm) return;
    const { kind, grant } = confirm;
    setConfirm(null);
    await run(grant.id, async () => {
      if (kind === 'leave') {
        await leaveShareGrant(grant.id);
        remove(grant.id);
        announce(t('standingShares.withMe.left', params(grant)));
      } else {
        await declineShareGrant(grant.id, { block: true });
        // The block ended every pending/active grant from this sender.
        await Promise.all([refresh(), refreshBlocks()]);
        announce(t('standingShares.withMe.blocked', { username: grant.from_username }));
      }
    });
  };

  const handleUnblock = async (b: ShareBlock) => {
    setBusyId(`block-${b.user_id}`);
    setActionError('');
    try {
      await deleteShareBlock(b.user_id);
      setBlocks((prev) => prev.filter((x) => x.user_id !== b.user_id));
      announce(t('standingShares.withMe.unblocked', { username: b.username }));
    } catch (err) {
      setActionError(t('standingShares.withMe.actionFailed', { message: getErrorMessage(err) }));
    } finally {
      setBusyId(null);
    }
  };

  const pending = items.filter((g) => g.status === 'pending');
  const active = items.filter((g) => g.status === 'active');

  const renderRow = (g: ShareGrantRecipientView) => {
    const busy = busyId === g.id;
    const p = params(g);
    return (
      <ListItem key={g.id} divider sx={{ display: 'block' }}>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 1 }}>
          <Typography variant="subtitle1" component="span" sx={{ overflowWrap: 'anywhere' }}>
            {g.label}
          </Typography>
          <Chip size="small" label={t(`standingShares.entityTypes.${g.entity_type}`)} />
        </Box>
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {t('standingShares.withMe.from', { username: g.from_username })}
        </Typography>
        {g.status === 'pending' ? (
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('standingShares.withMe.pendingNote')}
          </Typography>
        ) : (
          <>
            {g.entity_type === 'contact' && g.sections.length > 0 && (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('standingShares.withMe.sections', {
                  sections: g.sections
                    .map((s) => t(`settings.exportFieldPicker.sections.${s}`, { defaultValue: s }))
                    .join(', '),
                })}
              </Typography>
            )}
            {g.expires_at && (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('standingShares.withMe.expires', { date: formatDate(g.expires_at) })}
              </Typography>
            )}
          </>
        )}
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mt: 1 }}>
          {g.status === 'pending' ? (
            <>
              <Button
                size="small"
                variant="contained"
                disabled={busy}
                aria-label={t('standingShares.withMe.acceptLabel', p)}
                onClick={() => void handleAccept(g)}
              >
                {t('standingShares.withMe.accept')}
              </Button>
              <Button
                size="small"
                disabled={busy}
                aria-label={t('standingShares.withMe.declineLabel', p)}
                onClick={() => void handleDecline(g)}
              >
                {t('standingShares.withMe.decline')}
              </Button>
              <Button
                size="small"
                color="error"
                disabled={busy}
                aria-label={t('standingShares.withMe.blockLabel', { username: g.from_username })}
                onClick={() => setConfirm({ kind: 'block', grant: g })}
              >
                {t('standingShares.withMe.block')}
              </Button>
            </>
          ) : (
            <>
              <Button
                size="small"
                variant="contained"
                disabled={busy}
                aria-label={t('standingShares.withMe.viewLabel', p)}
                onClick={() => setViewing(g)}
              >
                {t('standingShares.withMe.view')}
              </Button>
              <Button
                size="small"
                color="error"
                disabled={busy}
                aria-label={t('standingShares.withMe.leaveLabel', p)}
                onClick={() => setConfirm({ kind: 'leave', grant: g })}
              >
                {t('standingShares.withMe.leave')}
              </Button>
            </>
          )}
        </Box>
      </ListItem>
    );
  };

  const empty = !loading && !error && items.length === 0;

  return (
    <Box sx={{ maxWidth: 800, mx: 'auto', mt: 2, px: 2, pb: 2 }}>
      <PageHeader title={t('standingShares.withMe.title')} />
      <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
        {t('standingShares.withMe.description')}
      </Typography>

      {loading && <LinearProgress sx={{ mb: 2 }} />}
      {error && <LoadError message={error} onRetry={() => void refresh()} sx={{ mb: 2 }} />}
      {actionError && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setActionError('')}>
          {actionError}
        </Alert>
      )}

      {empty && (
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {t('standingShares.withMe.empty')}
        </Typography>
      )}

      {pending.length > 0 && (
        <Box component="section" aria-labelledby="shared-pending-heading" sx={{ mb: 3 }}>
          <Typography id="shared-pending-heading" variant="h6" component="h2">
            {t('standingShares.withMe.pendingHeading')}
          </Typography>
          <List>{pending.map(renderRow)}</List>
        </Box>
      )}

      {active.length > 0 && (
        <Box component="section" aria-labelledby="shared-active-heading" sx={{ mb: 3 }}>
          <Typography id="shared-active-heading" variant="h6" component="h2">
            {t('standingShares.withMe.activeHeading')}
          </Typography>
          <List>{active.map(renderRow)}</List>
        </Box>
      )}

      {nextCursor && (
        <Box sx={{ display: 'flex', justifyContent: 'center', mb: 3 }}>
          <Button variant="outlined" onClick={() => void loadMore()} disabled={loading}>
            {t('common.loadMore')}
          </Button>
        </Box>
      )}

      <Box component="section" aria-labelledby="shared-blocked-heading">
        <Typography id="shared-blocked-heading" variant="h6" component="h2">
          {t('standingShares.withMe.blockedHeading')}
        </Typography>
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {t('standingShares.withMe.blockedHelp')}
        </Typography>
        {blocksError && (
          <LoadError message={blocksError} onRetry={() => void refreshBlocks()} sx={{ mt: 1 }} />
        )}
        {!blocksError && blocks.length === 0 && (
          <Typography variant="body2" sx={{ color: 'text.secondary', mt: 1 }}>
            {t('standingShares.withMe.blockedEmpty')}
          </Typography>
        )}
        {blocks.length > 0 && (
          <List>
            {blocks.map((b) => (
              <ListItem
                key={b.user_id}
                divider
                secondaryAction={
                  <Button
                    size="small"
                    disabled={busyId === `block-${b.user_id}`}
                    aria-label={t('standingShares.withMe.unblockLabel', { username: b.username })}
                    onClick={() => void handleUnblock(b)}
                  >
                    {t('standingShares.withMe.unblock')}
                  </Button>
                }
              >
                <Typography variant="body1">{b.username}</Typography>
              </ListItem>
            ))}
          </List>
        )}
      </Box>

      <ShareConfirmDialog
        open={confirm !== null}
        title={
          confirm?.kind === 'block'
            ? t('standingShares.withMe.blockTitle')
            : t('standingShares.withMe.leaveTitle')
        }
        body={
          confirm
            ? confirm.kind === 'block'
              ? t('standingShares.withMe.blockBody', { username: confirm.grant.from_username })
              : t('standingShares.withMe.leaveBody', params(confirm.grant))
            : ''
        }
        confirmLabel={
          confirm?.kind === 'block'
            ? t('standingShares.withMe.blockConfirm', {
                username: confirm.grant.from_username,
              })
            : t('standingShares.withMe.leave')
        }
        onConfirm={() => void handleConfirm()}
        onCancel={() => setConfirm(null)}
      />
      <SharedGrantContentDialog grant={viewing} onClose={() => setViewing(null)} />
    </Box>
  );
}
