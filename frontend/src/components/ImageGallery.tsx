import DeleteIcon from '@mui/icons-material/Delete';
import PhotoLibraryIcon from '@mui/icons-material/PhotoLibrary';
import {
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogContent,
  IconButton,
  Typography,
} from '@mui/material';
import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react';

// What a slot needs to drive the gallery's state: the owner wrappers (contact
// photos: menu, main image, crop, adopt) compose behaviour through this instead
// of forking the component.
export interface ImageGalleryContext<T> {
  items: T[];
  setItems: (update: (prev: T[]) => T[]) => void;
  busy: boolean;
  setError: (message: string) => void;
  setNotice: (message: string) => void;
  // Reloads the list from the owner's list endpoint.
  reload: () => Promise<void>;
  // Runs one write: busy flag, feedback reset, and a single logged error path.
  // The action reports failures to the user itself (setError) and rethrows.
  run: (action: () => Promise<void>, failure: string) => Promise<void>;
}

export interface ImageGalleryLabels {
  loading: string;
  alt: string;
  add: string;
  uploading: string;
  delete: string;
  loadFailed: string;
  uploadFailed: string;
  deleteFailed: string;
}

export interface ImageGalleryProps<T> {
  // Identity of the owner; a change reloads the list.
  ownerId: number;
  // Already-translated strings. The owner wrapper passes literal t('...') calls
  // so the i18n key-usage gate can still see every key (no dynamic prefix).
  labels: ImageGalleryLabels;
  // Names the owner in console diagnostics (e.g. "note").
  logName: string;
  list: () => Promise<T[]>;
  upload: (file: File) => Promise<unknown>;
  // When set, an editable gallery shows a delete button per image. Omit it when
  // renderActions owns the per-image actions.
  remove?: (image: T) => Promise<void>;
  imageKey: (image: T) => string;
  thumbnailSrc: (image: T) => string;
  fullSrc: (image: T) => string;
  // Read-only in the timeline; the owner's edit surface shows the actions.
  editable?: boolean;
  // Called after a successful add/delete so a parent can refetch a summary.
  onChanged?: () => void;
  // Called after a successful delete, in addition to onChanged.
  onRemoved?: () => void;
  itemTestId?: (image: T) => string;
  // Composition slots for owner-specific extras.
  renderOverlay?: (image: T) => ReactNode;
  renderActions?: (image: T, ctx: ImageGalleryContext<T>) => ReactNode;
  renderFooter?: (image: T) => ReactNode;
  renderExtras?: (ctx: ImageGalleryContext<T>) => ReactNode;
}

// ImageGallery is the one gallery for every image owner (activity, note,
// contact). A failed load is always shown, including on a read-only gallery;
// only an empty, error-free read-only gallery renders nothing.
export default function ImageGallery<T>({
  ownerId,
  labels,
  logName,
  list,
  upload,
  remove,
  imageKey,
  thumbnailSrc,
  fullSrc,
  editable = false,
  onChanged,
  onRemoved,
  itemTestId,
  renderOverlay,
  renderActions,
  renderFooter,
  renderExtras,
}: ImageGalleryProps<T>) {
  const [items, setItemsState] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [viewing, setViewing] = useState<T | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Callers pass fresh closures every render; reload on owner/locale change only.
  const listRef = useRef(list);
  listRef.current = list;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setItemsState(await listRef.current());
      setError('');
    } catch (err) {
      console.error(`Failed to load ${logName} images:`, err);
      setError(labels.loadFailed);
    } finally {
      setLoading(false);
    }
  }, [logName, labels.loadFailed]);

  useEffect(() => {
    void load();
  }, [load, ownerId]);

  const run = async (action: () => Promise<void>, failure: string) => {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await action();
    } catch (err) {
      console.error(failure, err);
    } finally {
      setBusy(false);
    }
  };

  const handleFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      for (const file of Array.from(files)) {
        await upload(file);
      }
      await load();
      onChanged?.();
    } catch (err) {
      console.error(`Failed to upload ${logName} image:`, err);
      setError(labels.uploadFailed);
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = '';
    }
  };

  const handleDelete = (image: T) =>
    run(async () => {
      try {
        await remove?.(image);
        setItemsState((prev) => prev.filter((item) => imageKey(item) !== imageKey(image)));
        onChanged?.();
        onRemoved?.();
      } catch (err) {
        setError(labels.deleteFailed);
        throw err;
      }
    }, `Failed to delete ${logName} image:`);

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 1 }}>
        <CircularProgress size={20} aria-label={labels.loading} />
      </Box>
    );
  }

  if (!editable && items.length === 0 && !error) return null;

  const ctx: ImageGalleryContext<T> = {
    items,
    setItems: (update) => setItemsState(update),
    busy,
    setError,
    setNotice,
    reload: load,
    run,
  };

  return (
    <Box sx={{ mt: 1.5 }}>
      {items.length > 0 && (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
          {items.map((image) => (
            <Box
              key={imageKey(image)}
              data-testid={itemTestId?.(image)}
              sx={{ position: 'relative', width: 88 }}
            >
              <Box
                component="img"
                src={thumbnailSrc(image)}
                alt={labels.alt}
                loading="lazy"
                onClick={() => setViewing(image)}
                sx={{
                  width: 88,
                  height: 88,
                  objectFit: 'cover',
                  borderRadius: 1,
                  cursor: 'zoom-in',
                  border: 1,
                  borderColor: 'divider',
                  display: 'block',
                }}
              />
              {renderOverlay?.(image)}
              {editable &&
                (renderActions ? (
                  renderActions(image, ctx)
                ) : remove ? (
                  <IconButton
                    size="small"
                    color="error"
                    aria-label={labels.delete}
                    disabled={busy}
                    onClick={() => void handleDelete(image)}
                    sx={{ position: 'absolute', top: -8, right: -8, bgcolor: 'background.paper' }}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                ) : null)}
              {renderFooter?.(image)}
            </Box>
          ))}
        </Box>
      )}

      {renderExtras?.(ctx)}

      {editable && (
        <Box sx={{ mt: items.length > 0 ? 1 : 0 }}>
          <input
            ref={inputRef}
            type="file"
            aria-label={labels.add}
            accept="image/jpeg,image/png,image/gif"
            multiple
            hidden
            onChange={(event) => void handleFiles(event.target.files)}
          />
          <Button
            size="small"
            startIcon={busy ? <CircularProgress size={16} /> : <PhotoLibraryIcon />}
            disabled={busy}
            onClick={() => inputRef.current?.click()}
          >
            {busy ? labels.uploading : labels.add}
          </Button>
        </Box>
      )}

      {error && (
        <Typography variant="caption" color="error" role="alert" sx={{ display: 'block', mt: 0.5 }}>
          {error}
        </Typography>
      )}
      {notice && (
        <Typography
          variant="caption"
          color="text.secondary"
          role="status"
          sx={{ display: 'block', mt: 0.5 }}
        >
          {notice}
        </Typography>
      )}

      <Dialog open={viewing !== null} onClose={() => setViewing(null)} maxWidth="lg">
        <DialogContent sx={{ p: 0, bgcolor: 'common.black' }}>
          {viewing && (
            <Box
              component="img"
              src={fullSrc(viewing)}
              alt={labels.alt}
              sx={{ maxWidth: '90vw', maxHeight: '90vh', display: 'block' }}
            />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}
