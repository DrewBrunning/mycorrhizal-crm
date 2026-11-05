import DeleteIcon from '@mui/icons-material/Delete';
import PhotoLibraryIcon from '@mui/icons-material/PhotoLibrary';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogContent,
  IconButton,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ActivityImage,
  activityImageSrc,
  deleteActivityImage,
  getActivityImages,
  patchActivityImage,
  uploadActivityImage,
} from '../api/activities';
import {
  type ContactImage,
  contactImageSrc,
  deleteContactImage,
  getContactImages,
  type ImageSensitivity,
  patchContactImage,
  uploadContactImage,
} from '../api/contactImages';
import {
  deleteNoteImage,
  getNoteImages,
  type NoteImage,
  noteImageSrc,
  patchNoteImage,
  uploadNoteImage,
} from '../api/notes';

/** The fields the gallery reads from any owner's image row. */
interface GalleryImage {
  id: number;
  file_url: string;
  thumbnail_url: string;
  sensitivity: ImageSensitivity;
}

const SENSITIVITIES: ImageSensitivity[] = ['normal', 'private', 'secret'];

// Literal keys so the i18n key-usage scan sees them.
const SENSITIVITY_LABELS = {
  normal: 'imageSensitivity.normal',
  private: 'imageSensitivity.private',
  secret: 'imageSensitivity.secret',
} as const;

/** One owner's API functions; the gallery itself is owner-agnostic. */
export interface ImageGalleryApi<I extends GalleryImage> {
  list: (ownerId: number) => Promise<{ images: I[] }>;
  upload: (ownerId: number, file: File) => Promise<unknown>;
  remove: (ownerId: number, imageId: number) => Promise<unknown>;
  /** PATCHes the allowlisted sensitivity and returns the updated row. */
  setSensitivity: (ownerId: number, imageId: number, sensitivity: ImageSensitivity) => Promise<I>;
  /** Resolves a server-relative file/thumbnail URL against the API origin. */
  src: (path: string) => string;
}

// Literal keys (not a template literal) so the i18n key-usage scan sees them.
const LABELS = {
  activityImages: {
    kind: 'activity',
    loading: 'activityImages.loading',
    alt: 'activityImages.alt',
    delete: 'activityImages.delete',
    add: 'activityImages.add',
    uploading: 'activityImages.uploading',
    loadFailed: 'activityImages.loadFailed',
    uploadFailed: 'activityImages.uploadFailed',
    deleteFailed: 'activityImages.deleteFailed',
  },
  noteImages: {
    kind: 'note',
    loading: 'noteImages.loading',
    alt: 'noteImages.alt',
    delete: 'noteImages.delete',
    add: 'noteImages.add',
    uploading: 'noteImages.uploading',
    loadFailed: 'noteImages.loadFailed',
    uploadFailed: 'noteImages.uploadFailed',
    deleteFailed: 'noteImages.deleteFailed',
  },
  contactImages: {
    kind: 'contact',
    loading: 'contactImages.loading',
    alt: 'contactImages.alt',
    delete: 'contactImages.delete',
    add: 'contactImages.add',
    uploading: 'contactImages.uploading',
    loadFailed: 'contactImages.loadFailed',
    uploadFailed: 'contactImages.uploadFailed',
    deleteFailed: 'contactImages.deleteFailed',
  },
} as const;

export type ImageGalleryPrefix = keyof typeof LABELS;

export const activityImageGalleryApi: ImageGalleryApi<ActivityImage> = {
  list: (id) => getActivityImages(id),
  upload: (id, file) => uploadActivityImage(id, file),
  remove: (id, imageId) => deleteActivityImage(id, imageId),
  setSensitivity: (id, imageId, sensitivity) => patchActivityImage(id, imageId, { sensitivity }),
  src: activityImageSrc,
};

export const noteImageGalleryApi: ImageGalleryApi<NoteImage> = {
  list: (id) => getNoteImages(id),
  upload: (id, file) => uploadNoteImage(id, file),
  remove: (id, imageId) => deleteNoteImage(id, imageId),
  setSensitivity: (id, imageId, sensitivity) => patchNoteImage(id, imageId, { sensitivity }),
  src: noteImageSrc,
};

export const contactImageGalleryApi: ImageGalleryApi<ContactImage> = {
  list: (id) => getContactImages(id),
  upload: (id, file) => uploadContactImage(id, file),
  remove: (id, imageId) => deleteContactImage(id, imageId),
  setSensitivity: (id, imageId, sensitivity) => patchContactImage(id, imageId, { sensitivity }),
  src: contactImageSrc,
};

interface ImageGalleryProps<I extends GalleryImage> {
  ownerId: number;
  api: ImageGalleryApi<I>;
  // Selects the owner's i18n namespace (activityImages / noteImages / contactImages).
  i18nPrefix: ImageGalleryPrefix;
  // Read-only in the timeline; an owner's edit surface shows add/remove.
  editable?: boolean;
  // Called after a successful add/delete so a parent can refetch a summary.
  onChanged?: () => void;
}

// ImageGallery renders an owner's images from the dedicated owner-nested
// endpoint. The server already returns ready-made relative URLs, so the client
// never guesses a file path (ADR 0040 decision 8). One component serves
// activities, notes and contacts (issue #2312); a load error is always shown,
// including read-only.
export default function ImageGallery<I extends GalleryImage>({
  ownerId,
  api,
  i18nPrefix,
  editable = false,
  onChanged,
}: ImageGalleryProps<I>) {
  const { t } = useTranslation();
  const labels = LABELS[i18nPrefix];
  const [images, setImages] = useState<I[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [viewing, setViewing] = useState<I | null>(null);
  const [sensitivityError, setSensitivityError] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.list(ownerId);
      setImages(response.images || []);
      setError('');
    } catch (err) {
      console.error(`Failed to load ${labels.kind} images:`, err);
      setError(t(labels.loadFailed));
    } finally {
      setLoading(false);
    }
  }, [api, ownerId, labels, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    setError('');
    try {
      for (const file of Array.from(files)) {
        await api.upload(ownerId, file);
      }
      await load();
      onChanged?.();
    } catch (err) {
      console.error(`Failed to upload ${labels.kind} image:`, err);
      setError(t(labels.uploadFailed));
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = '';
    }
  };

  const handleDelete = async (image: I) => {
    setBusy(true);
    try {
      await api.remove(ownerId, image.id);
      setImages((prev) => prev.filter((item) => item.id !== image.id));
      onChanged?.();
    } catch (err) {
      console.error(`Failed to delete ${labels.kind} image:`, err);
      setError(t(labels.deleteFailed));
    } finally {
      setBusy(false);
    }
  };

  const handleSensitivity = async (image: I, sensitivity: ImageSensitivity) => {
    setBusy(true);
    setSensitivityError('');
    try {
      const updated = await api.setSensitivity(ownerId, image.id, sensitivity);
      setImages((prev) => prev.map((item) => (item.id === image.id ? updated : item)));
      setViewing(updated);
      onChanged?.();
    } catch (err) {
      console.error(`Failed to update ${labels.kind} image sensitivity:`, err);
      setSensitivityError(t('imageSensitivity.failed'));
    } finally {
      setBusy(false);
    }
  };

  const closeViewer = () => {
    setViewing(null);
    setSensitivityError('');
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 1 }}>
        <CircularProgress size={20} aria-label={t(labels.loading)} />
      </Box>
    );
  }

  if (!editable && images.length === 0 && !error) return null;

  return (
    <Box sx={{ mt: 1.5 }}>
      {images.length > 0 && (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
          {images.map((image) => (
            <Box key={image.id} sx={{ position: 'relative' }}>
              <Box
                component="img"
                src={api.src(image.thumbnail_url)}
                alt={t(labels.alt)}
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
                }}
              />
              {editable && (
                <IconButton
                  size="small"
                  color="error"
                  aria-label={t(labels.delete)}
                  disabled={busy}
                  onClick={() => void handleDelete(image)}
                  sx={{ position: 'absolute', top: -8, right: -8, bgcolor: 'background.paper' }}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              )}
              {image.sensitivity !== 'normal' && (
                <Chip
                  size="small"
                  variant="outlined"
                  color={image.sensitivity === 'secret' ? 'error' : 'warning'}
                  label={t(SENSITIVITY_LABELS[image.sensitivity])}
                  sx={{ display: 'flex', mt: 0.25, maxWidth: 88 }}
                />
              )}
            </Box>
          ))}
        </Box>
      )}

      {editable && (
        <Box sx={{ mt: images.length > 0 ? 1 : 0 }}>
          <input
            ref={inputRef}
            type="file"
            aria-label={t(labels.add)}
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
            {busy ? t(labels.uploading) : t(labels.add)}
          </Button>
        </Box>
      )}

      {error && (
        <Typography variant="caption" color="error" sx={{ display: 'block', mt: 0.5 }}>
          {error}
        </Typography>
      )}

      <Dialog open={viewing !== null} onClose={closeViewer} maxWidth="lg">
        <DialogContent sx={{ p: 0, bgcolor: 'common.black' }}>
          {viewing && (
            <Box
              component="img"
              src={api.src(viewing.file_url)}
              alt={t(labels.alt)}
              sx={{ maxWidth: '90vw', maxHeight: '80vh', display: 'block', mx: 'auto' }}
            />
          )}
        </DialogContent>
        {viewing && editable && (
          <Box sx={{ p: 1.5 }}>
            <ToggleButtonGroup
              size="small"
              exclusive
              value={viewing.sensitivity}
              disabled={busy}
              aria-label={t('imageSensitivity.label')}
              onChange={(_event, value: ImageSensitivity | null) => {
                if (value && value !== viewing.sensitivity) void handleSensitivity(viewing, value);
              }}
            >
              {SENSITIVITIES.map((value) => (
                <ToggleButton key={value} value={value}>
                  {t(SENSITIVITY_LABELS[value])}
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.5, maxWidth: 480 }}
            >
              {t('imageSensitivity.help')}
            </Typography>
            {sensitivityError && (
              <Typography variant="caption" color="error" role="alert" sx={{ display: 'block' }}>
                {sensitivityError}
              </Typography>
            )}
          </Box>
        )}
      </Dialog>
    </Box>
  );
}
