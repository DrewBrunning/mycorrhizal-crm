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
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  type ActivityImage,
  activityImageSrc,
  deleteActivityImage,
  getActivityImages,
  uploadActivityImage,
} from '../api/activities';

interface ActivityImageGalleryProps {
  activityId: number;
  // Read-only in the timeline; an activity's edit surface shows add/remove.
  editable?: boolean;
  // Called after a successful add/delete so a parent can refetch a summary.
  onChanged?: () => void;
}

// ActivityImageGallery renders an activity's images from the dedicated
// owner-nested endpoint. The server already returns ready-made relative URLs,
// so the client never guesses a file path (ADR 0040 decision 8).
export default function ActivityImageGallery({
  activityId,
  editable = false,
  onChanged,
}: ActivityImageGalleryProps) {
  const { t } = useTranslation();
  const [images, setImages] = useState<ActivityImage[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [viewing, setViewing] = useState<ActivityImage | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const response = await getActivityImages(activityId);
      setImages(response.images || []);
      setError('');
    } catch (err) {
      console.error('Failed to load activity images:', err);
      setError(t('activityImages.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [activityId, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setBusy(true);
    setError('');
    try {
      for (const file of Array.from(files)) {
        await uploadActivityImage(activityId, file);
      }
      await load();
      onChanged?.();
    } catch (err) {
      console.error('Failed to upload activity image:', err);
      setError(t('activityImages.uploadFailed'));
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = '';
    }
  };

  const handleDelete = async (image: ActivityImage) => {
    setBusy(true);
    try {
      await deleteActivityImage(activityId, image.id);
      setImages((prev) => prev.filter((item) => item.id !== image.id));
      onChanged?.();
    } catch (err) {
      console.error('Failed to delete activity image:', err);
      setError(t('activityImages.deleteFailed'));
    } finally {
      setBusy(false);
    }
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 1 }}>
        <CircularProgress size={20} aria-label={t('activityImages.loading')} />
      </Box>
    );
  }

  if (!editable && images.length === 0) return null;

  return (
    <Box sx={{ mt: 1.5 }}>
      {images.length > 0 && (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
          {images.map((image) => (
            <Box key={image.id} sx={{ position: 'relative' }}>
              <Box
                component="img"
                src={activityImageSrc(image.thumbnail_url)}
                alt={t('activityImages.alt')}
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
                  aria-label={t('activityImages.delete')}
                  disabled={busy}
                  onClick={() => void handleDelete(image)}
                  sx={{ position: 'absolute', top: -8, right: -8, bgcolor: 'background.paper' }}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
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
            aria-label={t('activityImages.add')}
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
            {busy ? t('activityImages.uploading') : t('activityImages.add')}
          </Button>
        </Box>
      )}

      {error && (
        <Typography variant="caption" color="error" sx={{ display: 'block', mt: 0.5 }}>
          {error}
        </Typography>
      )}

      <Dialog open={viewing !== null} onClose={() => setViewing(null)} maxWidth="lg">
        <DialogContent sx={{ p: 0, bgcolor: 'common.black' }}>
          {viewing && (
            <Box
              component="img"
              src={activityImageSrc(viewing.file_url)}
              alt={t('activityImages.alt')}
              sx={{ maxWidth: '90vw', maxHeight: '90vh', display: 'block' }}
            />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}
