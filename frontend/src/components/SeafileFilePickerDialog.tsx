import { useState, useEffect, useCallback } from 'react';
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
import { SeafileLibrary, SeafileItem } from '../api/seafile';
import { getErrorMessage } from '../utils/errorHandler';

// SeafileLinkTarget is what the picker hands to the caller on selection: the
// browsed item plus the repo it lives in and its resolved repo-relative path.
export interface SeafileLinkTarget extends SeafileItem {
  repo_id: string;
  path: string;
}

interface SeafileFilePickerDialogProps {
  open: boolean;
  onClose: () => void;
  onFetchLibraries: () => Promise<SeafileLibrary[]>;
  onFetchDir: (repoId: string, path: string) => Promise<SeafileItem[]>;
  onSelect: (target: SeafileLinkTarget) => Promise<void>;
}

// SeafileFilePickerDialog is the L1 file/folder picker (P2b): pick a library,
// then navigate its folders, then choose a file or folder to link to the
// contact. The repo-relative path is what gets stored as the external_id.
export default function SeafileFilePickerDialog({
  open,
  onClose,
  onFetchLibraries,
  onFetchDir,
  onSelect,
}: SeafileFilePickerDialogProps) {
  const { t } = useTranslation();
  // Navigation state: no repo picked yet (null repoId) → libraries screen.
  const [libraries, setLibraries] = useState<SeafileLibrary[]>([]);
  const [repoId, setRepoId] = useState<string | null>(null);
  const [path, setPath] = useState('/');
  const [items, setItems] = useState<SeafileItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [selecting, setSelecting] = useState(false);

  const reset = useCallback(() => {
    setRepoId(null);
    setPath('/');
    setItems([]);
    setError('');
    setSelecting(false);
  }, []);

  useEffect(() => {
    if (!open) return;
    reset();
    setLibraries([]);
    setLoading(true);
    onFetchLibraries()
      .then((fetched) => setLibraries(fetched || []))
      .catch((err) => setError(getErrorMessage(err)))
      .finally(() => setLoading(false));
  }, [open, reset, onFetchLibraries]);

  const enterDir = async (nextRepoId: string, dirPath: string) => {
    setLoading(true);
    setError('');
    try {
      const fetched = await onFetchDir(nextRepoId, dirPath);
      setRepoId(nextRepoId);
      setPath(dirPath);
      setItems(fetched || []);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const handlePick = async (item: SeafileItem) => {
    if (repoId === null) return;
    setSelecting(true);
    try {
      await onSelect({ ...item, repo_id: repoId, path: joinDirPath(path, item.name) });
      onClose();
    } catch {
      setError(t('seafile.search.linkFailed'));
      setSelecting(false);
    }
  };

  // The path breadcrumb (e.g. "/ / Documents").
  const pathSegments = path.split('/').filter(Boolean);

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {repoId === null ? t('seafile.search.titleLibraries') : t('seafile.search.titleBrowse')}
      </DialogTitle>
      <DialogContent>
        {repoId !== null && (
          <Breadcrumbs sx={{ mb: 1.5 }} aria-label="breadcrumb">
            <MuiLink
              component="button"
              underline="hover"
              color="inherit"
              onClick={() => enterDir(repoId, '/')}
            >
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
                  onClick={() => enterDir(repoId, target)}
                >
                  {segment}
                </MuiLink>
              );
            })}
          </Breadcrumbs>
        )}

        {loading && <CircularProgress size={24} />}
        {!loading && error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && (
          <Box sx={{ maxHeight: 360, overflowY: 'auto' }}>
            <List dense>
              {repoId === null &&
                libraries.map((lib) => (
                  <ListItemButton key={lib.id} onClick={() => enterDir(lib.id, '/')}>
                    <ListItemIcon>
                      <FolderIcon />
                    </ListItemIcon>
                    <ListItemText primary={lib.name} secondary={lib.id} />
                  </ListItemButton>
                ))}
              {repoId !== null &&
                items.map((item) => (
                  <ListItemButton
                    key={item.name}
                    onClick={() => (item.type === 'dir' ? enterDir(repoId, joinDirPath(path, item.name)) : handlePick(item))}
                    disabled={selecting}
                  >
                    <ListItemIcon>
                      {item.type === 'dir' ? <FolderIcon /> : <InsertDriveFileIcon />}
                    </ListItemIcon>
                    <ListItemText
                      primary={item.name}
                      secondary={
                        item.type === 'file' ? formatFileSize(item.size ?? 0) : t('seafile.search.folder')
                      }
                    />
                  </ListItemButton>
                ))}
              {repoId !== null && items.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
                  {t('seafile.search.emptyFolder')}
                </Typography>
              )}
              {repoId === null && libraries.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
                  {t('seafile.search.noLibraries')}
                </Typography>
              )}
            </List>
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        {repoId !== null && (
          <Button onClick={() => enterDir(repoId, parentDir(path))} disabled={loading}>
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

function joinDirPath(path: string, name: string): string {
  const base = path === '/' ? '' : path;
  return base + '/' + name;
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
