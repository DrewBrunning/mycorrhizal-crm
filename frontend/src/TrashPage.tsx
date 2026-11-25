import {
  Avatar,
  Box,
  Button,
  Chip,
  LinearProgress,
  List,
  ListItem,
  ListItemAvatar,
  ListItemText,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from './api/client';
import type { TrashedContact, TrashedEntity } from './api/trash';
import LoadError from './components/LoadError';
import ShareConfirmDialog from './components/ShareConfirmDialog';
import { useSnackbar } from './context/SnackbarContext';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useFocusAfterRemoval } from './hooks/useFocusAfterRemoval';
import { useTrash } from './hooks/useTrash';
import { formatDateTime } from './timeFormat';
import { handleError } from './utils/errorHandler';

function displayName(item: TrashedContact): string {
  const c = item.contact;
  return [c.firstname, c.lastname].filter(Boolean).join(' ') || c.fn;
}

// Contact Trash (issue #2002, ADR 0037): restore or permanently delete the
// contacts the user deleted.
export default function TrashPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.trash'));
  const { showError, showSuccess, showWarning } = useSnackbar();
  const {
    items,
    entities,
    restoreEntity,
    retentionDays,
    loading,
    error,
    refresh,
    restore,
    purge,
    empty,
  } = useTrash();

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Restore and Delete forever unmount the row that held focus; hand it to a
  // neighbouring row's same button, else the page heading (#2918, #2920).
  const headingRef = useRef<HTMLHeadingElement>(null);
  const rows = useMemo(() => ({ items, entities }), [items, entities]);
  const restoreFocus = useFocusAfterRemoval(headingRef, rows);
  const purgeFocus = useFocusAfterRemoval(headingRef, items);
  const restoreOrder = [
    ...items.map((i) => `c${i.contact.id}`),
    ...entities.map((e) => `e${e.entity.id}`),
  ];

  // #2717: in-app, translated confirmations instead of window.confirm.
  const [confirm, setConfirm] = useState<
    { kind: 'purge'; item: TrashedContact } | { kind: 'empty' } | null
  >(null);

  const handleRestore = async (item: TrashedContact) => {
    restoreFocus.request(restoreOrder, `c${item.contact.id}`);
    try {
      const result = await restore(item.contact.id);
      const lost = result.skipped.reduce((n, s) => n + s.count, 0);
      if (lost > 0) {
        showWarning(
          t('trash.restoredWithSkipped', {
            name: displayName(item),
            count: lost,
          }),
        );
      } else {
        showSuccess(t('trash.restored', { name: displayName(item) }));
      }
    } catch (err) {
      restoreFocus.cancel();
      if (err instanceof ApiError && err.status === 409) {
        showError(t('trash.restoreConflict', { name: displayName(item) }));
      } else {
        handleError(err, { operation: 'restoring a contact' }, { showError });
      }
    }
  };

  const handleRestoreEntity = async (item: TrashedEntity) => {
    restoreFocus.request(restoreOrder, `e${item.entity.id}`);
    try {
      const result = await restoreEntity(item.entity.id);
      showSuccess(t('entities.deleted.restored', { name: item.entity.name }));
      if (result.same_name_live) {
        showWarning(t('entities.deleted.sameNameLive'));
      }
    } catch (err) {
      restoreFocus.cancel();
      handleError(err, { operation: 'restoring an entity' }, { showError });
    }
  };

  const handlePurge = async (item: TrashedContact) => {
    purgeFocus.request(
      items.map((i) => i.contact.id),
      item.contact.id,
    );
    try {
      await purge(item.contact.id);
      showSuccess(t('trash.purged', { name: displayName(item) }));
    } catch (err) {
      purgeFocus.cancel();
      handleError(err, { operation: 'permanently deleting a contact' }, { showError });
    }
  };

  const handleEmpty = async () => {
    try {
      const n = await empty();
      showSuccess(t('trash.emptied', { count: n }));
    } catch (err) {
      handleError(err, { operation: 'emptying the trash' }, { showError });
    }
  };

  return (
    <Box sx={{ maxWidth: 800, mx: 'auto', mt: 2, px: 2, pb: 2 }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          mb: 1,
        }}
      >
        <Typography variant="h5" component="h1" ref={headingRef} tabIndex={-1}>
          {t('trash.title')}
        </Typography>
        {items.length > 0 && (
          <Button color="error" variant="outlined" onClick={() => setConfirm({ kind: 'empty' })}>
            {t('trash.empty')}
          </Button>
        )}
      </Box>
      <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
        {retentionDays > 0
          ? t('trash.retention', { count: retentionDays })
          : t('trash.retentionForever')}
      </Typography>

      {loading && <LinearProgress sx={{ mb: 2 }} />}
      {error && <LoadError message={error} onRetry={() => void refresh()} sx={{ mb: 2 }} />}

      {/* The empty state stays OUT of the <List> (a <p> in a <ul> is invalid HTML). */}
      {items.length === 0 && entities.length === 0 && !loading && !error && (
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {t('trash.emptyState')}
        </Typography>
      )}

      {(items.length > 0 || entities.length > 0) && (
        <List>
          {items.map((item) => (
            <ListItem key={item.contact.id} divider sx={{ flexWrap: 'wrap', gap: 1 }}>
              <ListItemAvatar>
                <Avatar sx={{ bgcolor: 'primary.main' }}>
                  {(item.contact.firstname || item.contact.fn).charAt(0)}
                </Avatar>
              </ListItemAvatar>
              <ListItemText
                primary={displayName(item)}
                secondary={[
                  t('trash.deletedOn', {
                    date: formatDateTime(item.deleted_at),
                  }),
                  item.purge_at
                    ? t('trash.purgeOn', {
                        date: formatDateTime(item.purge_at),
                      })
                    : null,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              />
              <Box sx={{ display: 'flex', gap: 1 }}>
                <Button
                  size="small"
                  variant="contained"
                  aria-label={t('trash.restoreLabel', { name: displayName(item) })}
                  onClick={() => void handleRestore(item)}
                  ref={restoreFocus.register(`c${item.contact.id}`)}
                >
                  {t('trash.restore')}
                </Button>
                <Button
                  size="small"
                  color="error"
                  aria-label={t('trash.deleteForeverLabel', { name: displayName(item) })}
                  onClick={() => setConfirm({ kind: 'purge', item })}
                  ref={purgeFocus.register(item.contact.id)}
                >
                  {t('trash.deleteForever')}
                </Button>
              </Box>
            </ListItem>
          ))}
          {entities.map((item) => (
            <ListItem key={item.entity.id} divider sx={{ flexWrap: 'wrap', gap: 1 }}>
              <ListItemText
                primary={
                  <Box
                    component="span"
                    sx={{
                      display: 'inline-flex',
                      alignItems: 'center',
                      gap: 1,
                    }}
                  >
                    {item.entity.name}
                    <Chip size="small" label={t(`entities.kind.${item.entity.kind}`)} />
                  </Box>
                }
                secondary={[
                  item.merged_into_name
                    ? t('trash.mergedInto', { name: item.merged_into_name })
                    : null,
                  t('trash.deletedOn', {
                    date: formatDateTime(item.deleted_at),
                  }),
                  item.purge_at
                    ? t('trash.purgeOn', {
                        date: formatDateTime(item.purge_at),
                      })
                    : null,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              />
              <Button
                size="small"
                variant="contained"
                aria-label={t('trash.restoreLabel', { name: item.entity.name })}
                onClick={() => void handleRestoreEntity(item)}
                ref={restoreFocus.register(`e${item.entity.id}`)}
              >
                {t('trash.restore')}
              </Button>
            </ListItem>
          ))}
        </List>
      )}
      {entities.length > 0 && retentionDays > 0 && (
        <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mt: 1 }}>
          {t('trash.entityRetentionNote', { count: retentionDays })}
        </Typography>
      )}

      <ShareConfirmDialog
        open={confirm !== null}
        title={confirm?.kind === 'purge' ? t('trash.purgeTitle') : t('trash.emptyTitle')}
        body={
          confirm?.kind === 'purge'
            ? t('trash.purgeConfirm', { name: displayName(confirm.item) })
            : `${t('trash.emptyConfirm', { count: items.length })} ${t('trash.emptyScope')}`
        }
        confirmLabel={confirm?.kind === 'purge' ? t('trash.deleteForever') : t('trash.empty')}
        onCancel={() => setConfirm(null)}
        onConfirm={() => {
          const c = confirm;
          setConfirm(null);
          if (c?.kind === 'purge') void handlePurge(c.item);
          else if (c?.kind === 'empty') void handleEmpty();
        }}
      />
    </Box>
  );
}
