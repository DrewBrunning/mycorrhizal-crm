import AddIcon from '@mui/icons-material/Add';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import TuneIcon from '@mui/icons-material/Tune';
import {
  Alert,
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
  Stack,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { FieldDefinition, FieldDefinitionInput } from '../api/fieldDefinitions';
import { useSnackbar } from '../context/SnackbarContext';
import { useFieldDefinitions } from '../hooks/useFieldDefinitions';
import FieldDefinitionDialog from './FieldDefinitionDialog';

// T7's replacement for the v1 CustomFieldsSettings (which edited a plain list
// of names via /users/custom-fields). v2 definitions carry a type,
// constraints, sensitivity, and standards-projection target, so this is now a
// typed-definition manager: create/edit/delete a FieldDefinition.
export default function CustomFieldsSettings() {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const {
    definitions,
    loading,
    error,
    refresh,
    handleCreate,
    handleUpdate,
    handleDelete,
    handleMove,
  } = useFieldDefinitions();

  // Fetch the existing definitions on mount. Without this the list is always
  // empty until a create/edit/delete happens in the same session, so an
  // existing field could never be edited, deleted, or reordered (issue #1210's
  // reorder UI is the latest thing to depend on it). LinkFieldTypesSettings
  // does the same.
  useEffect(() => {
    void refresh();
  }, [refresh]);

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingDefinition, setEditingDefinition] = useState<FieldDefinition | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<FieldDefinition | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);

  const openCreate = () => {
    setEditingDefinition(null);
    setDialogOpen(true);
  };

  const openEdit = (def: FieldDefinition) => {
    setEditingDefinition(def);
    setDialogOpen(true);
  };

  const handleSave = async (input: FieldDefinitionInput) => {
    if (editingDefinition) {
      await handleUpdate(editingDefinition.id, input);
      showSuccess(t('customFields.updateSuccess'));
    } else {
      await handleCreate(input);
      showSuccess(t('customFields.createSuccess'));
    }
  };

  const handleDeleteConfirm = async () => {
    if (!deleteTarget) return;
    setDeleteBusy(true);
    try {
      await handleDelete(deleteTarget.id);
      showSuccess(t('customFields.deleteSuccess'));
      setDeleteTarget(null);
    } catch {
      // error surfaced by the hook's notifier path; keep the dialog open
      setDeleteTarget(null);
    } finally {
      setDeleteBusy(false);
    }
  };

  const sensitivityColor = (s: string) =>
    s === 'secret' ? 'error' : s === 'private' ? 'warning' : 'default';

  return (
    <>
      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
          <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
            <TuneIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
            <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
              {t('settings.customFields.title')}
            </Typography>
          </Box>
          <Divider sx={{ mb: 1.5 }} />

          <Stack spacing={1.5}>
            <Typography
              variant="body2"
              sx={{
                color: 'text.secondary',
              }}
            >
              {t('settings.customFields.description')}
            </Typography>

            {error && (
              <Alert severity="error" sx={{ py: 0 }}>
                {error}
              </Alert>
            )}

            {loading ? (
              <Typography
                variant="body2"
                sx={{
                  color: 'text.secondary',
                }}
              >
                {t('settings.customFields.loading')}
              </Typography>
            ) : (
              <>
                {definitions.length > 0 ? (
                  <List dense sx={{ py: 0 }}>
                    {definitions.map((def, index) => (
                      <ListItem
                        key={def.id}
                        sx={{ px: 0 }}
                        secondaryAction={
                          <>
                            <IconButton
                              size="small"
                              onClick={() => void handleMove(def.id, -1)}
                              disabled={index === 0}
                              aria-label={t('settings.customFields.moveUp', { name: def.label })}
                            >
                              <ArrowUpwardIcon fontSize="small" />
                            </IconButton>
                            <IconButton
                              size="small"
                              onClick={() => void handleMove(def.id, 1)}
                              disabled={index === definitions.length - 1}
                              aria-label={t('settings.customFields.moveDown', { name: def.label })}
                            >
                              <ArrowDownwardIcon fontSize="small" />
                            </IconButton>
                            <Button
                              size="small"
                              startIcon={<EditIcon fontSize="small" />}
                              onClick={() => openEdit(def)}
                            >
                              {t('common.edit')}
                            </Button>
                            <Button
                              size="small"
                              color="error"
                              startIcon={<DeleteIcon fontSize="small" />}
                              onClick={() => setDeleteTarget(def)}
                            >
                              {t('common.delete')}
                            </Button>
                          </>
                        }
                      >
                        <ListItemText
                          primary={def.label}
                          secondary={
                            <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap', mt: 0.5 }}>
                              <Chip
                                label={t(`customFields.types.${def.type}`)}
                                size="small"
                                variant="outlined"
                              />
                              {def.constraints?.multi && (
                                <Chip
                                  label={t('customFields.multiShort')}
                                  size="small"
                                  variant="outlined"
                                />
                              )}
                              <Chip
                                label={t(
                                  `customFields.sensitivity${def.sensitivity.charAt(0).toUpperCase()}${def.sensitivity.slice(1)}`,
                                )}
                                size="small"
                                color={sensitivityColor(def.sensitivity)}
                                variant="outlined"
                              />
                              {def.projection !== 'internal-only' && (
                                <Chip label={def.projection} size="small" variant="outlined" />
                              )}
                            </Box>
                          }
                        />
                      </ListItem>
                    ))}
                  </List>
                ) : (
                  <Typography
                    variant="body2"
                    sx={{
                      color: 'text.secondary',
                      fontStyle: 'italic',
                    }}
                  >
                    {t('settings.customFields.noFields')}
                  </Typography>
                )}

                <Box>
                  <Button
                    variant="contained"
                    color="primary"
                    size="small"
                    startIcon={<AddIcon />}
                    onClick={openCreate}
                  >
                    {t('settings.customFields.add')}
                  </Button>
                </Box>
              </>
            )}
          </Stack>
        </CardContent>
      </Card>

      <FieldDefinitionDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onSave={handleSave}
        definition={editingDefinition}
      />

      <Dialog open={!!deleteTarget} onClose={() => setDeleteTarget(null)}>
        <DialogTitle>{t('settings.customFields.deleteDialog.title')}</DialogTitle>
        <DialogContent>
          <DialogContentText>
            {t('settings.customFields.deleteDialog.message', {
              fieldName: deleteTarget?.label || '',
            })}
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteTarget(null)} disabled={deleteBusy}>
            {t('settings.customFields.deleteDialog.cancel')}
          </Button>
          <Button
            onClick={() => void handleDeleteConfirm()}
            color="error"
            disabled={deleteBusy}
            autoFocus
          >
            {t('settings.customFields.deleteDialog.confirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
