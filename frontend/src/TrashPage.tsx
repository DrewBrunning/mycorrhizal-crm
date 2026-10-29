import {
  Avatar,
  Box,
  Button,
  LinearProgress,
  List,
  ListItem,
  ListItemAvatar,
  ListItemText,
  Typography,
} from '@mui/material';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from './api/client';
import type { TrashedContact } from './api/trash';
import LoadError from './components/LoadError';
import { useSnackbar } from './context/SnackbarContext';
import { useDocumentTitle } from './hooks/useDocumentTitle';
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
  const { items, retentionDays, loading, error, refresh, restore, purge, empty } = useTrash();

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const handleRestore = async (item: TrashedContact) => {
    try {
      const result = await restore(item.contact.id);
      const lost = result.skipped.reduce((n, s) => n + s.count, 0);
      if (lost > 0) {
        showWarning(t('trash.restoredWithSkipped', { name: displayName(item), count: lost }));
      } else {
        showSuccess(t('trash.restored', { name: displayName(item) }));
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        showError(t('trash.restoreConflict', { name: displayName(item) }));
      } else {
        handleError(err, { operation: 'restoring a contact' }, { showError });
      }
    }
  };

  const handlePurge = async (item: TrashedContact) => {
    if (!window.confirm(t('trash.purgeConfirm', { name: displayName(item) }))) return;
    try {
      await purge(item.contact.id);
      showSuccess(t('trash.purged', { name: displayName(item) }));
    } catch (err) {
      handleError(err, { operation: 'permanently deleting a contact' }, { showError });
    }
  };

  const handleEmpty = async () => {
    if (!window.confirm(t('trash.emptyConfirm', { count: items.length }))) return;
    try {
      const n = await empty();
      showSuccess(t('trash.emptied', { count: n }));
    } catch (err) {
      handleError(err, { operation: 'emptying the trash' }, { showError });
    }
  };

  return (
    <Box sx={{ maxWidth: 800, mx: 'auto', mt: 2, px: 2, pb: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <Typography variant="h5" component="h1">
          {t('trash.title')}
        </Typography>
        {items.length > 0 && (
          <Button color="error" variant="outlined" onClick={() => void handleEmpty()}>
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
      {items.length === 0 && !loading && !error && (
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {t('trash.emptyState')}
        </Typography>
      )}

      {items.length > 0 && (
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
                  t('trash.deletedOn', { date: formatDateTime(item.deleted_at) }),
                  item.purge_at
                    ? t('trash.purgeOn', { date: formatDateTime(item.purge_at) })
                    : null,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              />
              <Box sx={{ display: 'flex', gap: 1 }}>
                <Button size="small" variant="contained" onClick={() => void handleRestore(item)}>
                  {t('trash.restore')}
                </Button>
                <Button size="small" color="error" onClick={() => void handlePurge(item)}>
                  {t('trash.deleteForever')}
                </Button>
              </Box>
            </ListItem>
          ))}
        </List>
      )}
    </Box>
  );
}
