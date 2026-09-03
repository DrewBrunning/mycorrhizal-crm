import AddIcon from '@mui/icons-material/Add';
import ContactsIcon from '@mui/icons-material/Contacts';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import SyncIcon from '@mui/icons-material/Sync';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Divider,
  FormControlLabel,
  IconButton,
  List,
  ListItem,
  ListItemText,
  Stack,
  Switch,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ContactSubscription,
  createContactSubscription,
  deleteContactSubscription,
  getContactSubscriptions,
  syncContactSubscription,
  updateContactSubscription,
} from '../api/contactSubscriptions';
import { daysSince, terminalReasonKey } from '../utils/syncHealth';

// CardDAV address-book subscriptions. The CalDAV analog is CalendarSyncSettings;
// this panel keeps the same shape minus the past/future-days window (no
// contacts equivalent). Terminal-failure + staleness surfacing is INT-04 (#467).

interface ContactFormState {
  name: string;
  url: string;
  username: string;
  password: string;
  sync_enabled: boolean;
}

const emptyForm = (): ContactFormState => ({
  name: '',
  url: '',
  username: '',
  password: '',
  sync_enabled: true,
});

export default function ContactSyncSettings() {
  const { t } = useTranslation();
  const [subscriptions, setSubscriptions] = useState<ContactSubscription[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editingHasPassword, setEditingHasPassword] = useState(false);
  const [editingUsername, setEditingUsername] = useState('');
  const [form, setForm] = useState<ContactFormState>(emptyForm());
  const [saving, setSaving] = useState(false);

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [toDelete, setToDelete] = useState<ContactSubscription | null>(null);
  const [deleting, setDeleting] = useState(false);

  const [syncing, setSyncing] = useState<Record<number, boolean>>({});

  const load = useCallback(async () => {
    try {
      setLoading(true);
      setSubscriptions(await getContactSubscriptions());
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.contactSync.loadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setEditingId(null);
    setEditingHasPassword(false);
    setEditingUsername('');
    setForm(emptyForm());
    setDialogOpen(true);
  };

  const openEdit = (sub: ContactSubscription) => {
    setEditingId(sub.id);
    setEditingHasPassword(sub.has_password);
    setEditingUsername(sub.username);
    setForm({
      name: sub.name,
      url: sub.url,
      username: sub.username,
      password: '',
      sync_enabled: sub.sync_enabled,
    });
    setDialogOpen(true);
  };

  const handleSave = async () => {
    setSaving(true);
    setError('');
    setSuccess('');
    const input = {
      name: form.name.trim(),
      url: form.url.trim(),
      username: form.username.trim(),
      password: form.password,
      clear_password:
        editingId !== null &&
        editingHasPassword &&
        editingUsername !== '' &&
        form.username.trim() === '' &&
        form.password === '',
      sync_enabled: form.sync_enabled,
    };
    try {
      if (editingId !== null) {
        const updated = await updateContactSubscription(editingId, input);
        setSubscriptions((prev) => prev.map((s) => (s.id === editingId ? updated : s)));
        setDialogOpen(false);
      } else {
        const created = await createContactSubscription(input);
        setSubscriptions((prev) => [...prev, created]);
        setDialogOpen(false);
        await handleSync(created);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.contactSync.saveError'));
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteConfirm = async () => {
    if (!toDelete) return;
    setDeleting(true);
    try {
      await deleteContactSubscription(toDelete.id);
      setSubscriptions((prev) => prev.filter((s) => s.id !== toDelete.id));
      setDeleteDialogOpen(false);
      setToDelete(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.contactSync.deleteError'));
      setDeleteDialogOpen(false);
    } finally {
      setDeleting(false);
    }
  };

  const handleSync = async (sub: ContactSubscription) => {
    setSyncing((prev) => ({ ...prev, [sub.id]: true }));
    setSuccess('');
    setError('');
    try {
      const result = await syncContactSubscription(sub.id);
      setSuccess(
        t('settings.contactSync.syncSuccess', {
          created: result.created,
          updated: result.updated,
          archived: result.archived,
          skipped: result.skipped,
        }),
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.contactSync.syncError'));
    } finally {
      setSyncing((prev) => ({ ...prev, [sub.id]: false }));
      await load();
    }
  };

  const formatLastSync = (sub: ContactSubscription) => {
    if (!sub.last_synced_at) return t('settings.contactSync.neverSynced');
    return t('settings.contactSync.lastSynced', {
      date: new Date(sub.last_synced_at).toLocaleString(),
    });
  };

  const staleLine = (lastSuccessAt: string | null) => {
    const days = daysSince(lastSuccessAt);
    if (days === null) return t('settings.syncHealth.neverSucceeded');
    if (days >= 2) return t('settings.syncHealth.staleDays', { days });
    return t('settings.syncHealth.staleRecent', {
      date: new Date(lastSuccessAt as string).toLocaleDateString(),
    });
  };

  const renderTerminalNotice = (sub: ContactSubscription) => {
    if (!sub.terminal_failure_at) return null;
    return (
      <Alert severity="error" sx={{ mt: 0.5, py: 0 }} icon={false}>
        <Typography variant="caption" sx={{ fontWeight: 600, display: 'block' }}>
          {t('settings.syncHealth.terminalTitle')}
        </Typography>
        <Typography variant="caption" component="span" sx={{ display: 'block' }}>
          {t(terminalReasonKey(sub.terminal_reason))}
        </Typography>
        <Typography variant="caption" component="span" sx={{ display: 'block' }}>
          {staleLine(sub.last_success_at)}
        </Typography>
      </Alert>
    );
  };

  const renderHealthLine = (sub: ContactSubscription) => {
    if (sub.terminal_failure_at) return null;
    if (sub.last_sync_status === 'success' && Object.keys(sub.last_run_stats).length > 0) {
      return (
        <Typography
          variant="caption"
          component="span"
          sx={{ color: 'text.secondary', display: 'block' }}
        >
          {t('settings.contactSync.healthLastRun', {
            created: sub.last_run_stats.created ?? 0,
            updated: sub.last_run_stats.updated ?? 0,
            archived: sub.last_run_stats.archived ?? 0,
            skipped: sub.last_run_stats.skipped ?? 0,
          })}
        </Typography>
      );
    }
    return null;
  };

  return (
    <>
      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Box
            sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <ContactsIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
              <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
                {t('settings.contactSync.title')}
              </Typography>
            </Box>
            <Button
              variant="contained"
              color="primary"
              size="small"
              startIcon={<AddIcon />}
              onClick={openCreate}
            >
              {t('settings.contactSync.add')}
            </Button>
          </Box>
          <Divider sx={{ mb: 1.5 }} />

          <Stack spacing={1.5}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {t('settings.contactSync.description')}
            </Typography>
            {error && (
              <Alert severity="error" sx={{ py: 0 }} onClose={() => setError('')}>
                {error}
              </Alert>
            )}
            {success && (
              <Alert severity="success" sx={{ py: 0 }} onClose={() => setSuccess('')}>
                {success}
              </Alert>
            )}

            {loading ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
                <CircularProgress size={24} />
              </Box>
            ) : subscriptions.length === 0 ? (
              <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                {t('settings.contactSync.empty')}
              </Typography>
            ) : (
              <List dense disablePadding>
                {subscriptions.map((sub) => (
                  <ListItem
                    key={sub.id}
                    divider
                    secondaryAction={
                      <Stack direction="row" spacing={0.5}>
                        <Tooltip title={t('settings.contactSync.syncNow')}>
                          <span>
                            <IconButton
                              size="small"
                              onClick={() => handleSync(sub)}
                              disabled={!!syncing[sub.id]}
                              aria-label={t('settings.contactSync.syncNow')}
                            >
                              {syncing[sub.id] ? (
                                <CircularProgress size={18} />
                              ) : (
                                <SyncIcon fontSize="small" />
                              )}
                            </IconButton>
                          </span>
                        </Tooltip>
                        <IconButton
                          size="small"
                          onClick={() => openEdit(sub)}
                          aria-label={t('common.edit')}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                        <IconButton
                          size="small"
                          onClick={() => {
                            setToDelete(sub);
                            setDeleteDialogOpen(true);
                          }}
                          aria-label={t('common.delete')}
                        >
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </Stack>
                    }
                  >
                    <ListItemText
                      slotProps={{ secondary: { component: 'div' } }}
                      primary={
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                          <Typography variant="body2" sx={{ fontWeight: 500 }}>
                            {sub.name}
                          </Typography>
                          {!sub.sync_enabled && (
                            <Chip size="small" label={t('settings.contactSync.disabled')} />
                          )}
                          {sub.last_sync_status === 'error' && !sub.terminal_failure_at && (
                            <Tooltip title={sub.last_sync_error}>
                              <Chip
                                size="small"
                                color="error"
                                label={
                                  sub.consecutive_failures > 1
                                    ? t('settings.contactSync.syncFailedCount', {
                                        count: sub.consecutive_failures,
                                      })
                                    : t('settings.contactSync.syncFailed')
                                }
                              />
                            </Tooltip>
                          )}
                        </Stack>
                      }
                      secondary={
                        <>
                          <Typography
                            variant="caption"
                            component="span"
                            sx={{ color: 'text.secondary', display: 'block' }}
                          >
                            {sub.url} — {formatLastSync(sub)}
                          </Typography>
                          {renderHealthLine(sub)}
                          {renderTerminalNotice(sub)}
                        </>
                      }
                    />
                  </ListItem>
                ))}
              </List>
            )}
          </Stack>
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>
          {editingId !== null
            ? t('settings.contactSync.editTitle')
            : t('settings.contactSync.addTitle')}
        </DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <Alert severity="info" sx={{ py: 0.5 }}>
              {t('settings.contactSync.dialogInfo')}
            </Alert>
            <TextField
              label={t('settings.contactSync.name')}
              value={form.name}
              onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
              fullWidth
              required
              size="small"
            />
            <TextField
              label={t('settings.contactSync.url')}
              value={form.url}
              onChange={(e) => setForm((prev) => ({ ...prev, url: e.target.value }))}
              fullWidth
              required
              size="small"
              placeholder="https://cloud.example.com/remote.php/dav/addressbooks/users/you/contacts/"
              helperText={t('settings.contactSync.urlHelp')}
            />
            {form.url.trim().toLowerCase().startsWith('http://') &&
              (form.username.trim() !== '' || form.password !== '' || editingHasPassword) && (
                <Alert severity="warning" sx={{ py: 0.5 }}>
                  {t('settings.contactSync.insecureUrlWarning')}
                </Alert>
              )}
            <Box
              sx={{ display: 'grid', gap: 1.5, gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr' } }}
            >
              <TextField
                label={t('settings.contactSync.username')}
                value={form.username}
                onChange={(e) => setForm((prev) => ({ ...prev, username: e.target.value }))}
                fullWidth
                size="small"
                helperText={t('settings.contactSync.credentialsOptional')}
              />
              <TextField
                label={t('settings.contactSync.password')}
                type="password"
                value={form.password}
                onChange={(e) => setForm((prev) => ({ ...prev, password: e.target.value }))}
                fullWidth
                size="small"
                autoComplete="new-password"
                helperText={
                  editingId !== null && editingHasPassword
                    ? t('settings.contactSync.passwordKeep')
                    : undefined
                }
              />
            </Box>
            <FormControlLabel
              control={
                <Switch
                  checked={form.sync_enabled}
                  onChange={(e) => setForm((prev) => ({ ...prev, sync_enabled: e.target.checked }))}
                />
              }
              label={t('settings.contactSync.autoSync')}
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>{t('common.cancel')}</Button>
          <Button
            variant="contained"
            onClick={handleSave}
            disabled={saving || !form.name.trim() || !form.url.trim()}
            startIcon={saving ? <CircularProgress size={16} color="inherit" /> : undefined}
          >
            {t('common.save')}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
        <DialogTitle>{t('settings.contactSync.deleteTitle')}</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {t('settings.contactSync.deleteConfirm', { name: toDelete?.name })}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>{t('common.cancel')}</Button>
          <Button
            color="error"
            variant="contained"
            onClick={handleDeleteConfirm}
            disabled={deleting}
          >
            {t('common.delete')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
