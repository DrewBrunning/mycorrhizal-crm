import { useState, useEffect } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  List,
  ListItemButton,
  ListItemText,
  ListItemIcon,
  Typography,
  CircularProgress,
  Alert,
  Breadcrumbs,
  Link as MuiLink,
} from '@mui/material';
import FolderIcon from '@mui/icons-material/Folder';
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile';
import AppDialog from './AppDialog';
import { useTranslation } from 'react-i18next';
import { WebDAVItem } from '../api/nextcloud';
import { getErrorMessage } from '../utils/errorHandler';

interface NextcloudFilePickerDialogProps {
  open: boolean;
  onClose: () => void;
  // Lists a directory's children (the dav root when path is undefined).
  onFetchDir: (path?: string) => Promise<WebDAVItem[]>;
  onSelect: (item: WebDAVItem) => Promise<void>;
}

// NextcloudFilePickerDialog is the L1 file/folder picker (P2c): navigate the
// user's Nextcloud/ownCloud dav root and pick a file or folder to link to the
// contact. The WebDAV path relative to the dav root is what gets stored as the
// external_id.
export default function NextcloudFilePickerDialog({
  open,
  onClose,
  onFetchDir,
  onSelect,
}: NextcloudFilePickerDialogProps) {
  const { t } = useTranslation();
  const [path, setPath] = useState('/');
  const [items, setItems] = useState<WebDAVItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selecting, setSelecting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setPath('/');
    setItems([]);
    setError('');
    setSelecting(false);
    setLoading(true);
    onFetchDir('/')
      .then((fetched) => setItems(fetched || []))
      .catch((err) => setError(getErrorMessage(err)))
      .finally(() => setLoading(false));
  }, [open, onFetchDir]);

  const enterDir = async (nextPath: string) => {
    setLoading(true);
    setError('');
    try {
      const fetched = await onFetchDir(nextPath);
      setPath(nextPath);
      setItems(fetched || []);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const handlePick = async (item: WebDAVItem) => {
    setSelecting(true);
    try {
      await onSelect(item);
      onClose();
    } catch {
      setError(t('nextcloud.search.linkFailed'));
      setSelecting(false);
    }
  };

  // The path breadcrumb (e.g. "/ / Documents").
  const pathSegments = (path === '/' ? '' : path).split('/').filter(Boolean);

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('nextcloud.search.title')}</DialogTitle>
      <DialogContent>
        <Breadcrumbs sx={{ mb: 1.5 }} aria-label="breadcrumb">
          <MuiLink component="button" underline="hover" color="inherit" onClick={() => enterDir('/')}>
            /
          </MuiLink>
          {pathSegments.map((segment, idx) => {
            const target = '/' + pathSegments.slice(0, idx + 1).join('/');
            const isLast = idx === pathSegments.length - 1;
            return isLast ? (
              <Typography key={target} color="text.primary">
                {segment}
              </Typography>
            ) : (
              <MuiLink
                key={target}
                component="button"
                underline="hover"
                color="inherit"
                onClick={() => enterDir(target)}
              >
                {segment}
              </MuiLink>
            );
          })}
        </Breadcrumbs>

        {loading && <CircularProgress size={24} />}
        {!loading && error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && (
          <Box sx={{ maxHeight: 360, overflowY: 'auto' }}>
            <List dense>
              {items.map((item) => (
                <ListItemButton
                  key={item.path}
                  onClick={() => (item.type === 'dir' ? enterDir(item.path) : handlePick(item))}
                  disabled={selecting}
                >
                  <ListItemIcon>
                    {item.type === 'dir' ? <FolderIcon /> : <InsertDriveFileIcon />}
                  </ListItemIcon>
                  <ListItemText
                    primary={item.name}
                    secondary={
                      item.type === 'file'
                        ? formatFileSize(item.size ?? 0)
                        : t('nextcloud.search.folder')
                    }
                  />
                </ListItemButton>
              ))}
              {items.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
                  {t('nextcloud.search.emptyFolder')}
                </Typography>
              )}
            </List>
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        {path !== '/' && (
          <Button onClick={() => enterDir(parentDir(path))} disabled={loading}>
            {t('common.back')}
          </Button>
        )}
        <Button onClick={onClose} disabled={selecting}>
          {t('common.cancel')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}

function parentDir(path: string): string {
  if (path === '/' || path === '') return '/';
  const trimmed = path.replace(/\/+$/, '');
  const idx = trimmed.lastIndexOf('/');
  if (idx <= 0) return '/';
  return trimmed.slice(0, idx);
}

function formatFileSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
