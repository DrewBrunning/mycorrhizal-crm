import { useState, useEffect, useCallback, FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import {
  Box,
  Typography,
  Card,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  IconButton,
  Chip,
  TablePagination,
  CircularProgress,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  FormControlLabel,
  Switch,
  Stack,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import AppDialog from './components/AppDialog';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import SendIcon from '@mui/icons-material/Send';
import AddIcon from '@mui/icons-material/Add';
import { getUsers, createUser, updateUser, deleteUser, triggerReminders } from './api/admin';
import { isAdmin } from './auth';
import { useSnackbar } from './context/SnackbarContext';
import type { User, UserUpdateInput, UserCreateInput } from './types';
import { useDocumentTitle } from './hooks/useDocumentTitle';

export default function UsersPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.users'));
  const navigate = useNavigate();
  const { showSuccess, showError } = useSnackbar();
  // A wide six-column table cannot be read at 360-414px, so below `sm` the
  // user list reflows to a stacked card-per-user layout (T32) — the same
  // single-column precedent T28 set for the contact page. The table stays for
  // tablets and up, where it is readable.
  const theme = useTheme();
  const compact = useMediaQuery(theme.breakpoints.down('sm'));
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [total, setTotal] = useState(0);
  const [triggerLoading, setTriggerLoading] = useState(false);

  // Create dialog state
  const emptyCreateForm: UserCreateInput = { username: '', email: '', password: '', is_admin: false };
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [createForm, setCreateForm] = useState<UserCreateInput>(emptyCreateForm);
  const [createLoading, setCreateLoading] = useState(false);
  const [createError, setCreateError] = useState('');

  // Edit dialog state
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [editForm, setEditForm] = useState<UserUpdateInput>({});
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState('');

  // Delete dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deletingUser, setDeletingUser] = useState<User | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState('');

  // Check admin access
  useEffect(() => {
    if (!isAdmin()) {
      navigate('/');
    }
  }, [navigate]);

  // Fetch users for the current page. Also called directly (not just from
  // the effect below) after a create, so the new user renders wherever the
  // server-paginated, id-ordered list actually places it rather than being
  // spliced onto whatever page happens to be showing (T39 review finding).
  const fetchUsers = useCallback(async () => {
    setLoading(true);
    setError('');

    try {
      const response = await getUsers(page + 1, rowsPerPage);
      setUsers(response.users);
      setTotal(response.total);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : t('users.loadError');
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  }, [page, rowsPerPage, t]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const handleChangePage = (_: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString();
  };

  // Create handlers
  const handleCreateClick = () => {
    setCreateForm(emptyCreateForm);
    setCreateError('');
    setCreateDialogOpen(true);
  };

  const handleCreateClose = () => {
    setCreateDialogOpen(false);
    setCreateForm(emptyCreateForm);
    setCreateError('');
  };

  const handleCreateSubmit = async (event: FormEvent) => {
    event.preventDefault();

    setCreateLoading(true);
    setCreateError('');

    try {
      await createUser(createForm);
      // Refetch the current page rather than splicing the new user into
      // local state — the list is server-paginated and ordered by id ASC,
      // so a locally-spliced row would render on the wrong page and desync
      // from the total TablePagination shows (T39 review finding).
      await fetchUsers();
      handleCreateClose();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : t('users.createError');
      setCreateError(errorMessage);
    } finally {
      setCreateLoading(false);
    }
  };

  // Edit handlers
  const handleEditClick = (user: User) => {
    setEditingUser(user);
    setEditForm({
      username: user.username,
      email: user.email,
      is_admin: user.is_admin,
    });
    setEditError('');
    setEditDialogOpen(true);
  };

  const handleEditClose = () => {
    setEditDialogOpen(false);
    setEditingUser(null);
    setEditForm({});
    setEditError('');
  };

  const handleEditSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (!editingUser) return;

    setEditLoading(true);
    setEditError('');

    try {
      // Only include fields that have changed
      const updateData: UserUpdateInput = {};
      if (editForm.username !== editingUser.username) {
        updateData.username = editForm.username;
      }
      if (editForm.email !== editingUser.email) {
        updateData.email = editForm.email;
      }
      if (editForm.password) {
        updateData.password = editForm.password;
      }
      if (editForm.is_admin !== editingUser.is_admin) {
        updateData.is_admin = editForm.is_admin;
      }

      const updatedUser = await updateUser(editingUser.id, updateData);
      setUsers(users.map(u => u.id === updatedUser.id ? updatedUser : u));
      handleEditClose();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : t('users.updateError');
      setEditError(errorMessage);
    } finally {
      setEditLoading(false);
    }
  };

  // Trigger reminders handler
  const handleTriggerReminders = async () => {
    setTriggerLoading(true);
    try {
      await triggerReminders();
      showSuccess(t('users.triggerRemindersSuccess'));
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : t('users.triggerRemindersError');
      showError(errorMessage);
    } finally {
      setTriggerLoading(false);
    }
  };

  // Delete handlers
  const handleDeleteClick = (user: User) => {
    setDeletingUser(user);
    setDeleteError('');
    setDeleteDialogOpen(true);
  };

  const handleDeleteClose = () => {
    setDeleteDialogOpen(false);
    setDeletingUser(null);
    setDeleteError('');
  };

  const handleDeleteConfirm = async () => {
    if (!deletingUser) return;

    setDeleteLoading(true);
    setDeleteError('');

    try {
      await deleteUser(deletingUser.id);
      setUsers(users.filter(u => u.id !== deletingUser.id));
      setTotal(total - 1);
      handleDeleteClose();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : t('users.deleteError');
      setDeleteError(errorMessage);
    } finally {
      setDeleteLoading(false);
    }
  };

  if (loading && users.length === 0) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1.5, gap: 1, flexWrap: 'wrap' }}>
        <Typography variant="h5" component="h1">
          {t('users.title')}
        </Typography>
        <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleCreateClick}
          >
            {t('users.addUser')}
          </Button>
          <Button
            variant="outlined"
            startIcon={triggerLoading ? <CircularProgress size={16} /> : <SendIcon />}
            onClick={handleTriggerReminders}
            disabled={triggerLoading}
          >
            {t('users.triggerReminders')}
          </Button>
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {compact ? (
        <>
          <Stack spacing={1.5}>
            {users.map((user) => (
              <Card key={user.id} sx={{ p: 1.5 }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    {/* #211: a per-item title in a list, not a page heading. */}
                    <Typography variant="subtitle1" component="p" sx={{ fontWeight: 600, overflowWrap: 'anywhere' }}>
                      {user.username}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
                      {user.email}
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5, flexWrap: 'wrap' }}>
                      <Chip
                        label={user.is_admin ? t('users.roles.admin') : t('users.roles.user')}
                        size="small"
                      />
                      <Typography variant="caption" color="text.secondary">
                        {t('users.columns.created')}: {formatDate(user.created_at)}
                      </Typography>
                    </Box>
                  </Box>
                  <Box sx={{ display: 'flex' }}>
                    <IconButton
                      size="small"
                      onClick={() => handleEditClick(user)}
                      title={t('common.edit')}
                      aria-label={t('common.edit')}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      onClick={() => handleDeleteClick(user)}
                      title={t('common.delete')}
                      aria-label={t('common.delete')}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </Box>
                </Box>
              </Card>
            ))}
          </Stack>
          {users.length === 0 && !loading && (
            <Typography color="text.secondary" align="center" sx={{ mt: 2 }}>
              {t('users.noUsers')}
            </Typography>
          )}
          <TablePagination
            component="div"
            count={total}
            page={page}
            onPageChange={handleChangePage}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={handleChangeRowsPerPage}
            rowsPerPageOptions={[10, 25, 50]}
          />
        </>
      ) : (
        <TableContainer component={Paper}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>{t('users.columns.id')}</TableCell>
                <TableCell>{t('users.columns.username')}</TableCell>
                <TableCell>{t('users.columns.email')}</TableCell>
                <TableCell>{t('users.columns.role')}</TableCell>
                <TableCell>{t('users.columns.created')}</TableCell>
                <TableCell align="right">{t('users.columns.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>{user.id}</TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>{user.username}</TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>{user.email}</TableCell>
                  <TableCell>
                    <Chip
                      label={user.is_admin ? t('users.roles.admin') : t('users.roles.user')}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>{formatDate(user.created_at)}</TableCell>
                  <TableCell align="right">
                    <IconButton
                      size="small"
                      onClick={() => handleEditClick(user)}
                      title={t('common.edit')}
                      aria-label={t('common.edit')}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      onClick={() => handleDeleteClick(user)}
                      title={t('common.delete')}
                      aria-label={t('common.delete')}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
              {users.length === 0 && !loading && (
                <TableRow>
                  <TableCell colSpan={6} align="center">
                    {t('users.noUsers')}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <TablePagination
            component="div"
            count={total}
            page={page}
            onPageChange={handleChangePage}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={handleChangeRowsPerPage}
            rowsPerPageOptions={[10, 25, 50]}
          />
        </TableContainer>
      )}

      {/* Create User Dialog */}
      <AppDialog open={createDialogOpen} onClose={handleCreateClose} maxWidth="sm" fullWidth>
        <form onSubmit={handleCreateSubmit}>
          <DialogTitle>{t('users.createDialog.title')}</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              {createError && <Alert severity="error">{createError}</Alert>}
              <TextField
                label={t('users.createDialog.username')}
                value={createForm.username}
                onChange={(e) => setCreateForm({ ...createForm, username: e.target.value })}
                fullWidth
                required
                size="small"
              />
              <TextField
                label={t('users.createDialog.email')}
                type="email"
                value={createForm.email}
                onChange={(e) => setCreateForm({ ...createForm, email: e.target.value })}
                fullWidth
                required
                size="small"
              />
              <TextField
                label={t('users.createDialog.password')}
                type="password"
                value={createForm.password}
                onChange={(e) => setCreateForm({ ...createForm, password: e.target.value })}
                fullWidth
                required
                size="small"
              />
              <FormControlLabel
                control={
                  <Switch
                    checked={createForm.is_admin || false}
                    onChange={(e) => setCreateForm({ ...createForm, is_admin: e.target.checked })}
                  />
                }
                label={t('users.createDialog.isAdmin')}
              />
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={handleCreateClose} disabled={createLoading}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" variant="contained" disabled={createLoading}>
              {createLoading ? t('common.saving') : t('common.save')}
            </Button>
          </DialogActions>
        </form>
      </AppDialog>

      {/* Edit User Dialog */}
      <AppDialog open={editDialogOpen} onClose={handleEditClose} maxWidth="sm" fullWidth>
        <form onSubmit={handleEditSubmit}>
          <DialogTitle>{t('users.editDialog.title')}</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              {editError && <Alert severity="error">{editError}</Alert>}
              <TextField
                label={t('users.editDialog.username')}
                value={editForm.username || ''}
                onChange={(e) => setEditForm({ ...editForm, username: e.target.value })}
                fullWidth
                size="small"
              />
              <TextField
                label={t('users.editDialog.email')}
                type="email"
                value={editForm.email || ''}
                onChange={(e) => setEditForm({ ...editForm, email: e.target.value })}
                fullWidth
                size="small"
              />
              <TextField
                label={t('users.editDialog.password')}
                type="password"
                value={editForm.password || ''}
                onChange={(e) => setEditForm({ ...editForm, password: e.target.value })}
                fullWidth
                size="small"
                helperText={t('users.editDialog.passwordHint')}
              />
              <FormControlLabel
                control={
                  <Switch
                    checked={editForm.is_admin || false}
                    onChange={(e) => setEditForm({ ...editForm, is_admin: e.target.checked })}
                  />
                }
                label={t('users.editDialog.isAdmin')}
              />
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={handleEditClose} disabled={editLoading}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" variant="contained" disabled={editLoading}>
              {editLoading ? t('common.saving') : t('common.save')}
            </Button>
          </DialogActions>
        </form>
      </AppDialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onClose={handleDeleteClose}>
        <DialogTitle>{t('users.deleteDialog.title')}</DialogTitle>
        <DialogContent>
          {deleteError && <Alert severity="error" sx={{ mb: 2 }}>{deleteError}</Alert>}
          <Typography>
            {t('users.deleteDialog.message', { username: deletingUser?.username })}
          </Typography>
          <Alert severity="warning" sx={{ mt: 2 }}>
            {t('users.deleteDialog.warning')}
          </Alert>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteClose} disabled={deleteLoading}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={handleDeleteConfirm}
            color="error"
            variant="contained"
            disabled={deleteLoading}
          >
            {deleteLoading ? t('common.delete') + '...' : t('common.delete')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
