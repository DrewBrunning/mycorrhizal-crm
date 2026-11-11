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
import type { TFunction } from 'i18next';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  listOutgoingShareGrants,
  reconfirmShareGrant,
  revokeShareGrant,
  type ShareGrantOwnerView,
} from './api/shareGrants';
import EditShareGrantDialog from './components/EditShareGrantDialog';
import LoadError from './components/LoadError';
import PageHeader from './components/PageHeader';
import ShareConfirmDialog from './components/ShareConfirmDialog';
import { useAnnouncer } from './context/AnnouncerContext';
import { useDateFormat } from './DateFormatProvider';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useShareGrantList } from './hooks/useShareGrantList';
import { getErrorMessage } from './utils/errorHandler';

type Confirm = { kind: 'revoke' | 'reconfirm'; grant: ShareGrantOwnerView };

const sectionLabel = (t: TFunction, s: string) =>
  t(`settings.exportFieldPicker.sections.${s}`, { defaultValue: s });

// "Shared by me": the owner side of standing shares (ADR 0042 decisions 5, 9, 10
// and 12; issue #2292). Status, expiry, revoke, re-confirm, narrow-only edit and
// the live circle member count. There is deliberately NO access history (no
// last-read time): the owner cannot see whether the recipient read anything.
// A grant whose recipient blocked the sender arrives from the API as "pending"
// and is shown exactly so.
export default function SharedByMePage() {
  const { t } = useTranslation();
  useDocumentTitle(t('standingShares.byMe.title'));
  const { announce } = useAnnouncer();
  const { formatDate } = useDateFormat();
  const { items, nextCursor, loading, error, refresh, loadMore, replace } =
    useShareGrantList<ShareGrantOwnerView>(listOutgoingShareGrants);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [actionError, setActionError] = useState('');
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [editing, setEditing] = useState<ShareGrantOwnerView | null>(null);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const params = (g: ShareGrantOwnerView) => ({ label: g.label, username: g.to_username });

  const handleConfirm = async () => {
    if (!confirm) return;
    const { kind, grant } = confirm;
    setConfirm(null);
    setBusyId(grant.id);
    setActionError('');
    try {
      if (kind === 'revoke') {
        replace(await revokeShareGrant(grant.id));
        announce(t('standingShares.byMe.revoked', params(grant)));
      } else {
        replace(await reconfirmShareGrant(grant.id));
        announce(t('standingShares.byMe.reconfirmed', params(grant)));
      }
    } catch (err) {
      setActionError(t('standingShares.byMe.actionFailed', { message: getErrorMessage(err) }));
    } finally {
      setBusyId(null);
    }
  };

  const renderRow = (g: ShareGrantOwnerView) => {
    const live = g.status === 'pending' || g.status === 'active';
    const busy = busyId === g.id;
    const p = params(g);
    return (
      <ListItem key={g.id} divider sx={{ display: 'block' }}>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 1 }}>
          <Typography variant="subtitle1" component="span" sx={{ overflowWrap: 'anywhere' }}>
            {g.label}
          </Typography>
          <Chip size="small" label={t(`standingShares.entityTypes.${g.entity_type}`)} />
          <Chip
            size="small"
            color={g.status === 'active' ? 'success' : 'default'}
            label={t(`standingShares.status.${g.status}`)}
          />
        </Box>
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {t('standingShares.byMe.to', { username: g.to_username })}
        </Typography>
        {g.entity_type === 'contact' && g.sections.length > 0 && (
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('standingShares.byMe.sections', {
              sections: g.sections.map((s) => sectionLabel(t, s)).join(', '),
            })}
          </Typography>
        )}
        {g.entity_type === 'circle' && g.member_count !== null && (
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('standingShares.byMe.members', { count: g.member_count })}
          </Typography>
        )}
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {g.expires_at
            ? t('standingShares.byMe.expires', { date: formatDate(g.expires_at) })
            : t('standingShares.byMe.noExpiry')}
        </Typography>
        {g.status === 'revoked' && g.revoked_reason && (
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t(`standingShares.byMe.revokedReason.${g.revoked_reason}`)}
          </Typography>
        )}
        {live && g.reconfirm_sections.length > 0 && (
          <Alert severity="warning" sx={{ mt: 1 }}>
            {t('standingShares.byMe.reconfirmNeeded', {
              username: g.to_username,
              sections: g.reconfirm_sections.map((s) => sectionLabel(t, s)).join(', '),
            })}
          </Alert>
        )}
        {live && (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mt: 1 }}>
            {g.reconfirm_sections.length > 0 && (
              <Button
                size="small"
                variant="contained"
                disabled={busy}
                aria-label={t('standingShares.byMe.reconfirmLabel', p)}
                onClick={() => setConfirm({ kind: 'reconfirm', grant: g })}
              >
                {t('standingShares.byMe.reconfirm')}
              </Button>
            )}
            <Button
              size="small"
              disabled={busy}
              aria-label={t('standingShares.byMe.editLabel', p)}
              onClick={() => setEditing(g)}
            >
              {t('standingShares.byMe.edit')}
            </Button>
            <Button
              size="small"
              color="error"
              disabled={busy}
              aria-label={t('standingShares.byMe.revokeLabel', p)}
              onClick={() => setConfirm({ kind: 'revoke', grant: g })}
            >
              {t('standingShares.byMe.revoke')}
            </Button>
          </Box>
        )}
      </ListItem>
    );
  };

  const empty = !loading && !error && items.length === 0;
  const reconfirm = confirm?.kind === 'reconfirm';

  return (
    <Box sx={{ maxWidth: 800, mx: 'auto', mt: 2, px: 2, pb: 2 }}>
      <PageHeader title={t('standingShares.byMe.title')} />
      <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
        {t('standingShares.byMe.description')}
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
          {t('standingShares.byMe.empty')}
        </Typography>
      )}
      {items.length > 0 && <List>{items.map(renderRow)}</List>}
      {nextCursor && (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 2 }}>
          <Button variant="outlined" onClick={() => void loadMore()} disabled={loading}>
            {t('common.loadMore')}
          </Button>
        </Box>
      )}
      <ShareConfirmDialog
        open={confirm !== null}
        title={
          reconfirm ? t('standingShares.byMe.reconfirmTitle') : t('standingShares.byMe.revokeTitle')
        }
        body={
          confirm
            ? reconfirm
              ? t('standingShares.byMe.reconfirmBody', {
                  username: confirm.grant.to_username,
                  sections: confirm.grant.reconfirm_sections
                    .map((s) => sectionLabel(t, s))
                    .join(', '),
                })
              : t('standingShares.byMe.revokeBody', { username: confirm.grant.to_username })
            : ''
        }
        confirmLabel={
          reconfirm ? t('standingShares.byMe.reconfirmConfirm') : t('standingShares.byMe.revoke')
        }
        onConfirm={() => void handleConfirm()}
        onCancel={() => setConfirm(null)}
      />
      <EditShareGrantDialog
        grant={editing}
        onClose={() => setEditing(null)}
        onSaved={(g) => {
          replace(g);
          setEditing(null);
          announce(t('standingShares.byMe.saved', { username: g.to_username }));
        }}
        onStale={() => void refresh()}
      />
    </Box>
  );
}
