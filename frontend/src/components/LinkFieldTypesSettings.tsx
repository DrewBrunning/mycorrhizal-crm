import AddIcon from '@mui/icons-material/Add';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import LinkIcon from '@mui/icons-material/Link';
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Divider,
  IconButton,
  List,
  ListItem,
  ListItemText,
  ListSubheader,
  MenuItem,
  Stack,
  SvgIcon,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type {
  LinkFieldType,
  LinkFieldTypeCategory,
  LinkFieldTypeInput,
} from '../api/linkFieldTypes';
import { useSnackbar } from '../context/SnackbarContext';
import { useLinkFieldTypes } from '../hooks/useLinkFieldTypes';
import { LINK_FIELD_TYPE_ICON_OPTIONS, resolveLinkFieldTypeIcon } from '../linkFieldTypeIcons';
import { getErrorMessage } from '../utils/errorHandler';

const CATEGORIES: LinkFieldTypeCategory[] = ['messaging', 'social', 'other'];

interface FormState {
  name: string;
  protocol: string;
  category: LinkFieldTypeCategory;
  icon: string;
}

const emptyForm = (): FormState => ({ name: '', protocol: '', category: 'messaging', icon: '' });

// Settings CRUD screen for the LinkFieldType registry (T34) — the
// "Contact field types" configuration surface the ticket describes.
// Self-contained like WebhooksSettings.tsx, dropped into SettingsPage.tsx.
export default function LinkFieldTypesSettings() {
  const { t } = useTranslation();
  const { showError } = useSnackbar();
  const {
    linkFieldTypes,
    loading,
    error,
    dialogOpen,
    editingLinkFieldType,
    refreshLinkFieldTypes,
    handleSaveLinkFieldType,
    handleAddLinkFieldType,
    handleEditLinkFieldType,
    handleDeleteLinkFieldType,
    handleMoveLinkFieldType,
    setDialogOpen,
  } = useLinkFieldTypes({ showError });

  useEffect(() => {
    refreshLinkFieldTypes();
  }, [refreshLinkFieldTypes]);

  const [form, setForm] = useState<FormState>(emptyForm());
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  const [deleteTarget, setDeleteTarget] = useState<LinkFieldType | null>(null);
  const [deleting, setDeleting] = useState(false);

  const openCreate = () => {
    setForm(emptyForm());
    setSaveError('');
    handleAddLinkFieldType();
  };

  const openEdit = (lt: LinkFieldType) => {
    setForm({ name: lt.name, protocol: lt.protocol, category: lt.category, icon: lt.icon || '' });
    setSaveError('');
    handleEditLinkFieldType(lt);
  };

  const handleSave = async () => {
    setSaving(true);
    setSaveError('');
    try {
      const input: LinkFieldTypeInput = {
        name: form.name.trim(),
        protocol: form.protocol.trim(),
        category: form.category,
        icon: form.icon.trim() || undefined,
      };
      await handleSaveLinkFieldType(input);
    } catch (err) {
      setSaveError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteConfirm = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await handleDeleteLinkFieldType(deleteTarget.id);
      setDeleteTarget(null);
    } finally {
      setDeleting(false);
    }
  };

  const grouped = CATEGORIES.map((category) => ({
    category,
    items: linkFieldTypes.filter((lt) => lt.category === category),
  })).filter((g) => g.items.length > 0);

  return (
    <>
      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Box
            sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <LinkIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
              <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
                {t('settings.linkFieldTypes.title')}
              </Typography>
            </Box>
            <Button
              variant="contained"
              color="primary"
              size="small"
              startIcon={<AddIcon />}
              onClick={openCreate}
            >
              {t('settings.linkFieldTypes.add')}
            </Button>
          </Box>
          <Divider sx={{ mb: 1.5 }} />

          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              {t('settings.linkFieldTypes.description')}
            </Typography>

            {error && (
              <Alert severity="error" sx={{ py: 0 }}>
                {error}
              </Alert>
            )}

            {loading ? (
              <Typography variant="body2" color="text.secondary">
                {t('settings.linkFieldTypes.loading')}
              </Typography>
            ) : linkFieldTypes.length === 0 ? (
              <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                {t('settings.linkFieldTypes.empty')}
              </Typography>
            ) : (
              grouped.map(({ category, items }) => (
                <List
                  key={category}
                  dense
                  sx={{ py: 0 }}
                  subheader={
                    <ListSubheader disableSticky sx={{ px: 0, lineHeight: 2 }}>
                      {t(`settings.linkFieldTypes.categoryOptions.${category}`)}
                    </ListSubheader>
                  }
                >
                  {items.map((lt, index) => (
                    <ListItem
                      key={lt.id}
                      sx={{ px: 0 }}
                      secondaryAction={
                        <Box sx={{ display: 'flex', gap: 0.5 }}>
                          <IconButton
                            size="small"
                            onClick={() => handleMoveLinkFieldType(lt.id, -1)}
                            disabled={index === 0}
                            aria-label={t('settings.linkFieldTypes.moveUp', { name: lt.name })}
                          >
                            <ArrowUpwardIcon fontSize="small" />
                          </IconButton>
                          <IconButton
                            size="small"
                            onClick={() => handleMoveLinkFieldType(lt.id, 1)}
                            disabled={index === items.length - 1}
                            aria-label={t('settings.linkFieldTypes.moveDown', { name: lt.name })}
                          >
                            <ArrowDownwardIcon fontSize="small" />
                          </IconButton>
                          <IconButton
                            size="small"
                            onClick={() => openEdit(lt)}
                            aria-label={t('settings.linkFieldTypes.edit', { name: lt.name })}
                          >
                            <EditIcon fontSize="small" />
                          </IconButton>
                          <IconButton
                            size="small"
                            onClick={() => setDeleteTarget(lt)}
                            color="error"
                            aria-label={t('settings.linkFieldTypes.delete', { name: lt.name })}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </Box>
                      }
                    >
                      <SvgIcon sx={{ mr: 1, color: 'text.secondary', fontSize: '1.2rem' }}>
                        <path d={resolveLinkFieldTypeIcon(lt.icon)} />
                      </SvgIcon>
                      <ListItemText
                        primaryTypographyProps={{ component: 'div' }}
                        secondaryTypographyProps={{ component: 'div' }}
                        primary={
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Typography variant="body2" sx={{ fontWeight: 500 }}>
                              {lt.name}
                            </Typography>
                            {lt.is_default && (
                              <Chip
                                size="small"
                                label={t('settings.linkFieldTypes.defaultBadge')}
                                sx={{ height: 18, fontSize: '0.7rem' }}
                              />
                            )}
                          </Box>
                        }
                        secondary={
                          <Typography variant="caption" color="text.secondary">
                            {lt.protocol || t('settings.linkFieldTypes.noProtocol')}
                          </Typography>
                        }
                      />
                    </ListItem>
                  ))}
                </List>
              ))
            )}
          </Stack>
        </CardContent>
      </Card>

      {/* Add/Edit dialog */}
      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>
          {editingLinkFieldType
            ? t('settings.linkFieldTypes.editTitle')
            : t('settings.linkFieldTypes.addTitle')}
        </DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            {saveError && <Alert severity="error">{saveError}</Alert>}
            <TextField
              label={t('settings.linkFieldTypes.name')}
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              size="small"
              required
              fullWidth
            />
            <TextField
              label={t('settings.linkFieldTypes.protocol')}
              value={form.protocol}
              onChange={(e) => setForm((f) => ({ ...f, protocol: e.target.value }))}
              size="small"
              fullWidth
              placeholder="https://example.com/{value}"
              helperText={t('settings.linkFieldTypes.protocolHelp')}
            />
            <TextField
              select
              label={t('settings.linkFieldTypes.category')}
              value={form.category}
              onChange={(e) =>
                setForm((f) => ({ ...f, category: e.target.value as LinkFieldTypeCategory }))
              }
              size="small"
              required
              fullWidth
            >
              {CATEGORIES.map((c) => (
                <MenuItem key={c} value={c}>
                  {t(`settings.linkFieldTypes.categoryOptions.${c}`)}
                </MenuItem>
              ))}
            </TextField>
            <Autocomplete
              freeSolo
              options={LINK_FIELD_TYPE_ICON_OPTIONS}
              value={form.icon}
              onInputChange={(_, value) => setForm((f) => ({ ...f, icon: value }))}
              renderOption={(props, option) => (
                <Box
                  component="li"
                  {...props}
                  sx={{ display: 'flex', alignItems: 'center', gap: 1 }}
                >
                  <SvgIcon fontSize="small">
                    <path d={resolveLinkFieldTypeIcon(option)} />
                  </SvgIcon>
                  {option}
                </Box>
              )}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label={t('settings.linkFieldTypes.icon')}
                  size="small"
                  helperText={t('settings.linkFieldTypes.iconHelp')}
                />
              )}
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>{t('common.cancel')}</Button>
          <Button onClick={handleSave} variant="contained" disabled={saving || !form.name.trim()}>
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog open={!!deleteTarget} onClose={() => setDeleteTarget(null)}>
        <DialogTitle>{t('settings.linkFieldTypes.deleteDialog.title')}</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {t('settings.linkFieldTypes.deleteDialog.message', { name: deleteTarget?.name ?? '' })}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteTarget(null)}>{t('common.cancel')}</Button>
          <Button onClick={handleDeleteConfirm} color="error" disabled={deleting} autoFocus>
            {t('common.delete')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
